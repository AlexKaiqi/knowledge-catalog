package testkit

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// GiteaEndpoint gives every caller in a test binary the same live Gitea
// instance: KC_GITEA_URL/KC_GITEA_TOKEN when set, otherwise a docker
// container started once and reused. run is a per-process nonce so
// concurrent test binaries (or reruns against the same env-provided server)
// do not collide on repository names.
//
// t.Skip is called (not t.Fatal) when neither docker nor an env endpoint is
// available: acceptance against a real Gitea is opt-in, not a hard CI
// requirement for the rest of the suite.
func GiteaEndpoint(t *testing.T) (base, token, run string) {
	t.Helper()
	giteaOnce.Do(func() {
		if u := strings.TrimSpace(os.Getenv("KC_GITEA_URL")); u != "" {
			giteaURL = strings.TrimRight(u, "/")
			giteaToken = strings.TrimSpace(os.Getenv("KC_GITEA_TOKEN"))
			if giteaToken == "" {
				giteaErr = fmt.Errorf("KC_GITEA_URL set but KC_GITEA_TOKEN empty")
			}
			giteaRun = fmt.Sprintf("%d", time.Now().UnixNano())
			return
		}
		if _, err := exec.LookPath("docker"); err != nil {
			giteaErr = fmt.Errorf("docker not available")
			return
		}
		giteaURL, giteaToken, giteaErr = startDockerGitea()
		giteaRun = fmt.Sprintf("%d", time.Now().UnixNano())
	})
	if giteaErr != nil {
		t.Skip(giteaErr.Error())
	}
	if giteaURL == "" || giteaToken == "" {
		t.Skip("gitea not available")
	}
	return giteaURL, giteaToken, giteaRun
}

const giteaDockerImage = "gitea/gitea:1.26.3"

var (
	giteaOnce  sync.Once
	giteaURL   string
	giteaToken string
	giteaErr   error
	giteaRun   string
)

// startDockerGitea runs one container per test binary (process), named and
// ported by pid: go test ./... runs packages as separate processes, and each
// package importing testkit has its own sync.Once, so a fixed name/port would
// race across packages (one process's docker rm -f killing another's
// mid-test — this is exactly the CONNECTION RESET flakiness a shared name
// produced before this was per-process).
//
// Known limitation: nothing stops the container when the test binary exits
// (sync.Once spans every test in the package, so there is no single test
// whose Cleanup would be the right moment). Containers accumulate across many
// `go test ./...` runs; `docker container prune` or a CI reset clears them.
// The previous fixed name self-limited to one leftover container instead —
// trading that for freedom from the cross-package race is the right side of
// this tradeoff.
func startDockerGitea() (string, string, error) {
	name := fmt.Sprintf("kc-gitea-t12-%d", os.Getpid())
	port, err := freeTCPPort()
	if err != nil {
		return "", "", fmt.Errorf("find a free port for gitea: %w", err)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	_ = exec.Command("docker", "rm", "-f", name).Run()
	run := exec.Command("docker", "run", "-d", "--rm", "--name", name,
		"-p", fmt.Sprintf("%d:3000", port),
		"-e", "GITEA__security__INSTALL_LOCK=true",
		"-e", "GITEA__database__DB_TYPE=sqlite3",
		"-e", "GITEA__server__DOMAIN=127.0.0.1",
		"-e", "GITEA__server__HTTP_PORT=3000",
		"-e", "GITEA__server__ROOT_URL="+base+"/",
		"-e", "GITEA__server__START_SSH_SERVER=false",
		"-e", "GITEA__service__DISABLE_REGISTRATION=true",
		"-e", "GITEA__repository__DEFAULT_BRANCH=main",
		giteaDockerImage,
	)
	if out, err := run.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("docker run gitea: %v: %s", err, out)
	}
	if err := waitGitea(base); err != nil {
		logs, _ := exec.Command("docker", "logs", "--tail", "80", name).CombinedOutput()
		return "", "", fmt.Errorf("%v\n%s", err, logs)
	}
	if err := createGiteaUser(name); err != nil {
		return "", "", err
	}
	token, err := mintGiteaToken(name)
	if err != nil {
		return "", "", err
	}
	return base, token, nil
}

// freeTCPPort asks the OS for an ephemeral port and releases it immediately;
// docker binds it a moment later. A small race is possible but not one this
// suite has hit in practice, and it is the same trick net/http/httptest uses.
func freeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitGitea(base string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/api/v1/version")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == 200 && strings.Contains(string(body), "version") {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("gitea %s did not become ready", base)
}

func dockerGitea(name string, args ...string) (string, error) {
	full := append([]string{"exec", "-u", "git", name, "gitea"}, args...)
	out, err := exec.Command("docker", full...).CombinedOutput()
	if err != nil {
		full = append([]string{"exec", name, "gitea"}, args...)
		out, err = exec.Command("docker", full...).CombinedOutput()
	}
	return string(out), err
}

func createGiteaUser(name string) error {
	deadline := time.Now().Add(60 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		out, err := dockerGitea(name, "admin", "user", "create",
			"--admin", "--username", "kc", "--password", "kcpass123",
			"--email", "kc@local", "--must-change-password=false")
		last = out
		if err == nil || strings.Contains(out, "already") {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("gitea user create: %s", last)
}

func mintGiteaToken(name string) (string, error) {
	attempts := [][]string{
		{"admin", "user", "generate-access-token", "--username", "kc", "--token-name", "t12", "--raw", "--scopes", "all"},
		{"admin", "user", "generate-access-token", "--username", "kc", "--token-name", "t12-write", "--raw", "--scopes", "write:repository,write:user,write:organization"},
		{"admin", "user", "generate-access-token", "--username", "kc", "--token-name", "t12-plain", "--raw"},
	}
	var last string
	for _, args := range attempts {
		out, err := dockerGitea(name, args...)
		last = out
		if err != nil {
			continue
		}
		token := strings.TrimSpace(out)
		if i := strings.LastIndex(token, "\n"); i >= 0 {
			token = strings.TrimSpace(token[i+1:])
		}
		if token != "" && !strings.Contains(token, " ") {
			return token, nil
		}
	}
	return "", fmt.Errorf("gitea token: %s", last)
}

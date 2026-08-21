package gitea_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"kc/gitea"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

const dockerImage = "gitea/gitea:1.26.3"

var (
	giteaOnce  sync.Once
	giteaURL   string
	giteaToken string
	giteaErr   error
	giteaRun   string
)

func giteaEndpoint(t *testing.T) (base, token string) {
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
	return giteaURL, giteaToken
}

func startDockerGitea() (string, string, error) {
	name := "kc-gitea-t12"
	base := "http://127.0.0.1:3011"
	_ = exec.Command("docker", "rm", "-f", name).Run()
	run := exec.Command("docker", "run", "-d", "--name", name,
		"-p", "3011:3000",
		"-e", "GITEA__security__INSTALL_LOCK=true",
		"-e", "GITEA__database__DB_TYPE=sqlite3",
		"-e", "GITEA__server__DOMAIN=127.0.0.1",
		"-e", "GITEA__server__HTTP_PORT=3000",
		"-e", "GITEA__server__ROOT_URL="+base+"/",
		"-e", "GITEA__server__START_SSH_SERVER=false",
		"-e", "GITEA__service__DISABLE_REGISTRATION=true",
		"-e", "GITEA__repository__DEFAULT_BRANCH=main",
		dockerImage,
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

func TestT12GiteaContract(t *testing.T) {
	base, token := giteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)
	factory := func(t *testing.T, id string) repository.Repository {
		t.Helper()
		sum := sha256.Sum256([]byte(id + giteaRun))
		name := "kc-" + hex.EncodeToString(sum[:8])
		dsn := base + "/kc/" + name
		repo, err := gitea.Open(kernel.RepositoryID(id), dsn, token)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	// SnapshotStore + Knowledge surface: see testkit.RepositoryContract.
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}

func TestGiteaReadPinnedCommitNotWorktree(t *testing.T) {
	base, token := giteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)
	id := kernel.RepositoryID("kr://conformance/gitea/pin")
	sum := sha256.Sum256([]byte(string(id) + giteaRun + "pin"))
	dsn := base + "/kc/kc-" + hex.EncodeToString(sum[:8])
	repo, err := gitea.Open(id, dsn, token)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, repo, "refs/heads/main")
	first, err := repo.ApplyCommit(testkit.CommitChange(id, root, "policy/P-1", map[string]any{"version": 1}, "policies/P-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ApplyCommit(testkit.CommitChange(id, first, "policy/P-1", map[string]any{"version": 2}, "policies/P-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	v1, err := repo.Read("policy/P-1", first)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := repo.Read("policy/P-1", second)
	if err != nil {
		t.Fatal(err)
	}
	if testkitAsInt(v1.Value.(map[string]any)["version"]) != 1 {
		t.Fatalf("pinned first %#v", v1.Value)
	}
	if testkitAsInt(v2.Value.(map[string]any)["version"]) != 2 {
		t.Fatalf("live second %#v", v2.Value)
	}
	if _, ok := any(repo).(interface{ RootDir() string }); ok {
		t.Fatal("gitea adapter must not expose a worktree RootDir")
	}
}

func testkitAsInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return -1
	}
}

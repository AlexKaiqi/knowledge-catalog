package dolt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func (r *DoltRepository) run(args ...string) (string, error) {
	return r.runWithInput("", args...)
}

// runSQLScript keeps potentially large mutations out of argv. This matters
// for the Docker fallback where execve and tini both enforce argument-size
// limits long before a bounded native knowledge batch becomes large.
func (r *DoltRepository) runSQLScript(script string) (string, error) {
	return r.runWithInput(script, "sql")
}

func (r *DoltRepository) runWithInput(input string, args ...string) (string, error) {
	if err := r.verifyManaged(); err != nil {
		return "", err
	}
	// A command of its own needs the database write lease, which a live query
	// session holds. Release it here so mutations are never refused as
	// read-only and Dolt keeps exactly one active writer.
	closeEngineAt(r.rootDir)
	cmd := doltCommand(r.rootDir, doltBinary(), input != "", args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("dolt %s: %s", strings.Join(args, " "), text)
	}
	return stripANSI(text), nil
}

func (r *DoltRepository) verifyManaged() error {
	if r.managedAllocation == "" {
		return nil
	}
	return VerifyManaged(r.rootDir, r.repositoryID, r.managedAllocation)
}

// doltBinary names the executable KC_DOLT_BIN selected, or "" when the
// binary is resolved from PATH or the Docker fallback is used.
func doltBinary() string {
	return strings.TrimSpace(os.Getenv("KC_DOLT_BIN"))
}

// doltCommand builds one engine invocation. The Docker fallback lets the
// reference implementation exercise a real engine without a host binary;
// stdin is attached when the caller streams statements to it.
func doltCommand(rootDir, bin string, stdin bool, args ...string) *exec.Cmd {
	cmd, _ := doltInvocation(rootDir, bin, stdin, "", args...)
	return cmd
}

func doltInvocation(rootDir, bin string, stdin bool, container string, args ...string) (*exec.Cmd, string) {
	forceDocker := strings.TrimSpace(os.Getenv("KC_DOLT_FORCE_DOCKER")) == "1"
	if bin != "" {
		cmd := exec.Command(bin, args...)
		cmd.Dir = rootDir
		return cmd, ""
	}
	if found, err := exec.LookPath("dolt"); err == nil && !forceDocker {
		cmd := exec.Command(found, args...)
		cmd.Dir = rootDir
		return cmd, ""
	}
	image := strings.TrimSpace(os.Getenv("KC_DOLT_DOCKER_IMAGE"))
	if image == "" {
		image = doltDockerImage
	}
	dockerArgs := []string{"run", "--rm"}
	if container != "" {
		dockerArgs = append(dockerArgs, "--name", container)
	}
	dockerArgs = append(dockerArgs, "-u", dockerUser(), "-e", "HOME=/tmp")
	if stdin {
		dockerArgs = append(dockerArgs, "-i")
	}
	dockerArgs = append(dockerArgs,
		"-v", rootDir+":/repo", "-w", "/repo",
		"--entrypoint", "/bin/sh", image, "-c",
		"dolt config --global --add user.email kc@localhost >/dev/null 2>&1; "+
			"dolt config --global --add user.name kc >/dev/null 2>&1; "+
			"exec dolt \"$@\" 2>&1",
		"dolt")
	return exec.Command("docker", append(dockerArgs, args...)...), container
}

func dockerSessionName(rootDir string) string {
	sum := sha256.Sum256([]byte(rootDir))
	return "kc-dolt-session-" + hex.EncodeToString(sum[:12])
}

func removeDockerContainer(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
}

func dockerUser() string {
	return strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
}

func stripANSI(value string) string {
	for {
		start := strings.IndexByte(value, 0x1b)
		if start < 0 {
			return strings.TrimSpace(value)
		}
		end := strings.IndexByte(value[start:], 'm')
		if end < 0 {
			return strings.TrimSpace(value[:start])
		}
		value = value[:start] + value[start+end+1:]
	}
}

type doltRows struct {
	Rows []map[string]any `json:"rows"`
}

// query runs one immutable or metadata statement. It reuses this database's
// live session so a bounded read does not pay an engine start per statement.
func (r *DoltRepository) query(query string) ([]map[string]any, error) {
	if err := r.requireDurableAuthority(); err != nil {
		return nil, err
	}
	if err := r.verifyManaged(); err != nil {
		return nil, err
	}
	out, err := engineQuery(r.rootDir, doltBinary(), query)
	if err == errEngineUnusable {
		// This environment answered nothing on the session transport; a
		// one-shot process still produces the same result.
		out, err = r.run("sql", "-r", "json", "-q", query)
	}
	if err != nil {
		return nil, err
	}
	out = stripANSI(out)
	var result doltRows
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("decode Dolt JSON: %w (%s)", err, out)
	}
	return result.Rows, nil
}

func sqlString(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

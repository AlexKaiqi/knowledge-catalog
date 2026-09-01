package cli

// Host mount daemon lifecycle for kcfs: spawning the detached child, waiting
// for its fixed readiness manifest, and stopping it. It owns no protocol
// semantics; the immutable file plan is built in workspacefs.go.

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"kc/kernel"
)

type daemonMountResult struct {
	workspaceFSManifest
	PID int `json:"pid"`
}

// daemonMountWorkspaceFS is the synchronous host-controller boundary: it
// returns only after the child has mounted every path and emitted its fixed
// manifest. DSH can therefore veto session creation without racing an
// asynchronous stdout listener.
func daemonMountWorkspaceFS(argv []string, stdout, stderr io.Writer) int {
	// Reject shape errors in the synchronous controller before starting a
	// detached child. Otherwise a malformed request has no stdout manifest and
	// the controller can only discover it after the readiness timeout.
	if _, err := parseWorkspaceFSConfig("daemon-mount", argv, io.Discard); err != nil {
		writeWorkspaceFSError(stderr, err)
		return 2
	}
	executable, err := os.Executable()
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	cmd := exec.Command(executable, append([]string{"mount"}, argv...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	logFile, err := os.CreateTemp("", "kcfs-daemon-*.log")
	if err != nil {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	logPath := logFile.Name()
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = os.Remove(logPath)
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	pidLogPath := workspaceFSDaemonLogPath(cmd.Process.Pid)
	if err := os.Rename(logPath, pidLogPath); err != nil {
		_ = logFile.Close()
		stopDaemonMountChild(cmd)
		_ = os.Remove(logPath)
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	logPath = pidLogPath
	_ = logFile.Close()
	type readyResult struct {
		manifest workspaceFSManifest
		err      error
	}
	ready := make(chan readyResult, 1)
	go func() {
		manifest, err := decodeWorkspaceFSReady(pipe)
		if err != nil {
			ready <- readyResult{err: err}
			return
		}
		ready <- readyResult{manifest: manifest}
	}()
	select {
	case result := <-ready:
		if result.err != nil {
			stopDaemonMountChild(cmd)
			writeDaemonMountLog(stderr, logPath)
			_ = os.Remove(logPath)
			writeWorkspaceFSError(stderr, result.err)
			return 1
		}
		pid := cmd.Process.Pid
		if err := cmd.Process.Release(); err != nil {
			stopDaemonMountChild(cmd)
			writeDaemonMountLog(stderr, logPath)
			_ = os.Remove(logPath)
			writeWorkspaceFSError(stderr, err)
			return 1
		}
		writeWorkspaceFSJSON(stdout, daemonMountResult{workspaceFSManifest: result.manifest, PID: pid})
		return 0
	case <-time.After(30 * time.Second):
		stopDaemonMountChild(cmd)
		writeDaemonMountLog(stderr, logPath)
		_ = os.Remove(logPath)
		writeWorkspaceFSError(stderr, kernel.Fail(kernel.ErrTemporaryUnavailable, "kcfs mount did not become ready within 30s"))
		return 1
	}
}

func workspaceFSDaemonLogPath(pid int) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("kcfs-daemon-%d.log", pid))
}

func writeDaemonMountLog(stderr io.Writer, logPath string) {
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return
	}
	_, _ = stderr.Write(content)
	if content[len(content)-1] != '\n' {
		_, _ = io.WriteString(stderr, "\n")
	}
}

func decodeWorkspaceFSReady(reader io.Reader) (workspaceFSManifest, error) {
	var manifest workspaceFSManifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return workspaceFSManifest{}, fmt.Errorf("decode kcfs ready manifest: %w", err)
	}
	return manifest, nil
}

func stopDaemonMountChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func stopWorkspaceFS(argv []string, stderr io.Writer) int {
	set := flag.NewFlagSet("kcfs stop", flag.ContinueOnError)
	set.SetOutput(stderr)
	pid := set.Int("pid", 0, "daemon mount process id")
	if err := set.Parse(argv); err != nil || *pid <= 1 || set.NArg() != 0 {
		if err == nil {
			err = fmt.Errorf("kcfs stop requires one valid --pid")
		}
		writeWorkspaceFSError(stderr, err)
		return 2
	}
	if err := syscall.Kill(*pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		writeWorkspaceFSError(stderr, err)
		return 1
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(*pid, 0); err == syscall.ESRCH {
			_ = os.Remove(workspaceFSDaemonLogPath(*pid))
			return 0
		}
		time.Sleep(25 * time.Millisecond)
	}
	writeWorkspaceFSError(stderr, kernel.Fail(kernel.ErrTemporaryUnavailable, "kcfs process %d did not stop within 10s", *pid))
	return 1
}

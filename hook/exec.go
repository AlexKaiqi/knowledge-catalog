package hook

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kc/kernel"
)

const execTimeout = 5 * time.Second

func runExec(home, run string, stdin []byte) error {
	path, err := lookPath(home, run)
	if err != nil {
		return kernel.Fail(kernel.ErrHookDenied, "hook %s: %s", run, err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.Dir = home
	cmd.Stdin = bytes.NewReader(append(append([]byte{}, stdin...), '\n'))
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return kernel.Fail(kernel.ErrHookDenied, "hook timed out: %s", run)
	}
	if err != nil {
		msg := string(bytes.TrimSpace(out))
		if msg == "" {
			msg = err.Error()
		}
		return kernel.Fail(kernel.ErrHookDenied, "hook %s: %s", run, msg)
	}
	return nil
}

func lookPath(home, run string) (string, error) {
	path := run
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, run)
	}
	cleaned := filepath.Clean(path)
	rel, err := filepath.Rel(filepath.Clean(home), cleaned)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", kernel.Fail(kernel.ErrHookDenied, "hook --run must stay under workspace home")
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", os.ErrNotExist
	}
	return cleaned, nil
}

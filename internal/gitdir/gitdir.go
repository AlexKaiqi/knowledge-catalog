// Package gitdir provides git plumbing for a directory with a working tree.
// It knows commits, refs and file bytes, but not knowledge or Workspace types.
package gitdir

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const DefaultBranch = "main"

// Dir is one git working directory. Methods shell out to git.
type Dir struct {
	root string
}

// At wraps an existing directory without touching it.
func At(root string) *Dir { return &Dir{root: root} }

// Open creates an empty repository with a root commit when needed.
func Open(root string, excludes ...string) (*Dir, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	d := At(root)
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
			return nil, err
		}
		_, _ = d.Git("branch", "-M", DefaultBranch)
		if _, err := d.Commit(Signature{Message: "root"}, true); err != nil {
			return nil, err
		}
	}
	if err := d.Exclude(excludes...); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Dir) Root() string { return d.root }

// Git runs one git command and returns trimmed stdout.
func (d *Dir) Git(args ...string) (string, error) {
	out, err := d.gitRaw(args...)
	return strings.TrimSpace(string(out)), err
}

func (d *Dir) gitRaw(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = d.root
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
				err = fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
			}
		}
	}
	return out, err
}

func (d *Dir) OK(args ...string) bool {
	_, err := d.Git(args...)
	return err == nil
}

// Rev resolves a ref, or HEAD when ref is empty.
func (d *Dir) Rev(ref string) (string, bool) {
	if ref == "" {
		ref = "HEAD"
	}
	if !d.OK("rev-parse", "--verify", ref) {
		return "", false
	}
	out, err := d.Git("rev-parse", ref)
	if err != nil {
		return "", false
	}
	return out, true
}

func (d *Dir) HasCommit(commit string) bool {
	return commit != "" && d.OK("cat-file", "-e", commit+"^{commit}")
}

func (d *Dir) Config(key string) (string, error) {
	return d.Git("config", "--local", "--get", key)
}

func (d *Dir) SetConfig(key, value string) error {
	_, err := d.Git("config", "--local", key, value)
	return err
}

// Exclude appends patterns to .git/info/exclude.
func (d *Dir) Exclude(patterns ...string) error {
	if len(patterns) == 0 {
		return nil
	}
	path := filepath.Join(d.root, ".git", "info", "exclude")
	current, _ := os.ReadFile(path)
	text := string(current)
	changed := false
	for _, pattern := range patterns {
		if strings.Contains("\n"+text+"\n", "\n"+pattern+"\n") {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += pattern + "\n"
		changed = true
	}
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

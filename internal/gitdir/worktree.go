package gitdir

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (d *Dir) CheckedOutRef() string {
	ref, _ := d.Git("symbolic-ref", "-q", "HEAD")
	return ref
}

func (d *Dir) Checkout(name string) error {
	_, err := d.Git("checkout", "-q", name)
	return err
}

// AddWorktree creates a linked, detached working tree at commit.
func (d *Dir) AddWorktree(dest, commit string) error {
	_, err := d.Git("worktree", "add", "--detach", "-q", dest, commit)
	return err
}

func (d *Dir) RemoveWorktree(dest string) error {
	_, err := d.Git("worktree", "remove", "--force", dest)
	return err
}

func (d *Dir) CheckoutDetached(commit string) error {
	_, err := d.Git("checkout", "--detach", "-q", commit)
	return err
}

func (d *Dir) Dirty() bool {
	out, _ := d.Git("status", "--porcelain")
	return out != ""
}

// SparseCheckout limits this working tree to subPath.
func (d *Dir) SparseCheckout(subPath string) error {
	subPath = strings.Trim(subPath, "/")
	if subPath == "" || subPath == "." {
		return nil
	}
	if _, err := d.Git("config", "core.sparseCheckout", "true"); err != nil {
		return err
	}
	path, err := d.Git("rev-parse", "--git-path", "info/sparse-checkout")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(d.root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := "/" + subPath + "/\n/" + subPath + "/**\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	_, err = d.Git("read-tree", "-mu", "HEAD")
	return err
}

// ResetHard moves HEAD and the worktree to commit.
func (d *Dir) ResetHard(commit string) error {
	_, err := d.Git("reset", "--hard", "-q", commit)
	return err
}

type WorktreeChange struct {
	Path    string
	Orig    string
	Removed bool
}

func (d *Dir) PorcelainChanges() ([]WorktreeChange, error) {
	raw, err := d.Git("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(raw), nil
}

func parsePorcelain(raw string) []WorktreeChange {
	var out []WorktreeChange
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" || len(line) < 4 {
			continue
		}
		xy := line[:2]
		rest := strings.TrimPrefix(line[2:], " ")
		path, orig := rest, ""
		if xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C' {
			if from, to, ok := strings.Cut(rest, " -> "); ok {
				orig, path = from, to
			}
		}
		path = unquotePorcelain(path)
		orig = unquotePorcelain(orig)
		removed := xy[0] == 'D' || xy[1] == 'D'
		if orig != "" {
			out = append(out, WorktreeChange{Path: orig, Removed: true}, WorktreeChange{Path: path})
			continue
		}
		out = append(out, WorktreeChange{Path: path, Removed: removed})
	}
	return out
}

func unquotePorcelain(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
}

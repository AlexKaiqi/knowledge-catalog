package gitdir

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenCache creates only a local object cache; it creates no root commit.
func OpenCache(root string) (*Dir, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	d := At(root)
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		if _, err := d.Git("init", "-q", "--initial-branch="+DefaultBranch); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// RemoteRef queries the authority without consulting cached refs. An absent
// ref returns an empty commit; transport/authentication failures return errors.
func (d *Dir) RemoteRef(remote, ref string) (string, error) {
	cmd := exec.Command("git", "ls-remote", "--exit-code", "--refs", "--", remote, ref)
	cmd.Dir = d.root
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 2 {
			return "", nil
		}
		return "", fmt.Errorf("git authority lookup failed: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("git authority returned no matching ref")
}

// FetchRef obtains immutable objects without changing a local branch.
func (d *Dir) FetchRef(remote, ref string) (string, error) {
	cmd := exec.Command("git", "fetch", "--quiet", "--no-tags", "--", remote, ref)
	cmd.Dir = d.root
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git authority fetch failed: %w", err)
	}
	head, ok := d.Rev("FETCH_HEAD")
	if !ok {
		return "", fmt.Errorf("git fetch returned no commit")
	}
	return head, nil
}

// PushCAS publishes one candidate using the authority's expected-old check.
// A lost acknowledgement is accepted only when the authority confirms next.
func (d *Dir) PushCAS(remote, ref, next, expected string) error {
	cmd := exec.Command("git", "push", "--quiet", "--force-with-lease="+ref+":"+expected, "--", remote, next+":"+ref)
	cmd.Dir = d.root
	if err := cmd.Run(); err != nil {
		actual, lookupErr := d.RemoteRef(remote, ref)
		if lookupErr != nil {
			return fmt.Errorf("git authority publication could not be confirmed: %w", lookupErr)
		}
		if actual == next {
			return nil
		}
		if actual != expected {
			return ErrMoved{Ref: ref, Expected: expected, Actual: actual}
		}
		return fmt.Errorf("git authority rejected publication: %w", err)
	}
	return nil
}

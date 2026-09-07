package gitdir

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// BuildFlatCommit writes immutable objects without touching a ref, index, or
// worktree. The caller publishes the candidate with CompareAndSwap or PushCAS.
func (d *Dir) BuildFlatCommit(parent string, files map[string][]byte, sig Signature) (string, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
			return "", fmt.Errorf("flat tree requires a file name")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var entries bytes.Buffer
	for _, name := range names {
		blob, err := d.gitInput(files[name], "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&entries, "100644 blob %s\t%s\x00", blob, name)
	}
	tree, err := d.gitInput(entries.Bytes(), "mktree", "-z")
	if err != nil {
		return "", err
	}
	return d.buildTreeCommit(tree, parent, sig)
}

func (d *Dir) buildTreeCommit(tree, parent string, sig Signature) (string, error) {
	name, email, message := sig.Format()
	args := []string{"-c", "user.name=" + name, "-c", "user.email=" + email, "commit-tree", tree, "-m", message}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	return d.Git(args...)
}

func (d *Dir) gitInput(input []byte, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = d.root
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CompareAndSwap is the only local ref publication point. An empty expected
// requires the ref not to exist. Failed publication leaves the ref untouched.
func (d *Dir) CompareAndSwap(ref, next, expected string) error {
	_, err := d.Git("update-ref", ref, next, expected)
	if err != nil {
		actual, _ := d.Rev(ref)
		if actual != expected {
			return ErrMoved{Ref: ref, Expected: expected, Actual: actual}
		}
	}
	return err
}

// Materialize updates disposable files and the index, never a ref. Failure is
// a cache failure and cannot undo an already accepted authority commit.
func (d *Dir) Materialize(commit string) error {
	_, err := d.Git("read-tree", "--reset", "-u", commit)
	return err
}

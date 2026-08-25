package gitdir

import "fmt"

func (d *Dir) StageAll() error {
	_, err := d.Git("add", "-A")
	return err
}

// Commit records the staged tree. It does not stage.
func (d *Dir) Commit(sig Signature, allowEmpty bool) (string, error) {
	name, email, message := sig.Format()
	args := []string{"-c", "user.name=" + name, "-c", "user.email=" + email, "commit", "-q", "-m", message}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	if _, err := d.Git(args...); err != nil {
		return "", err
	}
	head, ok := d.Rev("")
	if !ok {
		return "", fmt.Errorf("commit in %s left no HEAD", d.root)
	}
	return head, nil
}

// CommitWorktree stages and commits the worktree onto the default branch with CAS.
func (d *Dir) CommitWorktree(expected string, sig Signature) (string, error) {
	current, ok := d.Rev(BranchRef(DefaultBranch))
	if !ok {
		return "", fmt.Errorf("branch %s does not exist in %s", DefaultBranch, d.root)
	}
	if expected != "" && current != expected {
		return "", ErrMoved{Ref: BranchRef(DefaultBranch), Expected: expected, Actual: current}
	}
	if err := d.Checkout(DefaultBranch); err != nil {
		return "", err
	}
	if err := d.StageAll(); err != nil {
		return "", err
	}
	if !d.Dirty() {
		return current, nil
	}
	return d.Commit(sig, false)
}

type ErrMoved struct {
	Ref      string
	Expected string
	Actual   string
}

func (e ErrMoved) Error() string {
	return fmt.Sprintf("ref %s moved: expected commit %s, actual %s", e.Ref, e.Expected, e.Actual)
}

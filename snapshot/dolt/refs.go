package dolt

import (
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

func (r *DoltRepository) queryHash(ref string) (kernel.CommitID, error) {
	rows, err := r.query("SELECT DOLT_HASHOF(" + sqlString(ref) + ") AS hash")
	if err != nil || len(rows) != 1 {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "Dolt ref %s does not exist", ref)
	}
	hash, _ := rows[0]["hash"].(string)
	if hash == "" {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "Dolt ref %s does not exist", ref)
	}
	return kernel.CommitID(hash), nil
}

func doltBranch(ref string) (string, bool) {
	switch {
	case ref == "", ref == "HEAD", ref == snapshot.DefaultRef:
		return "main", true
	case ref == "refs/kc/archived":
		return "kc-archived", true
	case strings.HasPrefix(ref, "refs/heads/"):
		name := strings.TrimPrefix(ref, "refs/heads/")
		return name, name != "" && !strings.Contains(name, "..")
	default:
		return "", false
	}
}

func (r *DoltRepository) Head(ref string) (kernel.CommitID, error) {
	commit, ok := r.GetRef(ref)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", ref)
	}
	return commit, nil
}

func (r *DoltRepository) GetRef(ref string) (kernel.CommitID, bool) {
	branch, ok := doltBranch(ref)
	if !ok {
		return "", false
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	commit, err := r.queryHash(branch)
	return commit, err == nil
}

func (r *DoltRepository) HasCommit(commit kernel.CommitID) bool {
	if commit == "" {
		return false
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	_, err := r.queryHash(string(commit))
	return err == nil
}

func (r *DoltRepository) CreateRef(ref string, commit kernel.CommitID) error {
	branch, ok := doltBranch(ref)
	if !ok || branch == "main" {
		return kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", ref)
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	if ref != "refs/kc/archived" && r.archivedLocked() {
		return kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	if _, err := r.queryHash(branch); err == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "ref %s already exists", ref)
	}
	if _, err := r.queryHash(string(commit)); err != nil {
		return kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	_, err := r.run("branch", branch, string(commit))
	return err
}

func (r *DoltRepository) Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	if r.Archived() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	branch, ok := doltBranch(targetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", targetRef)
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	current, err := r.queryHash(branch)
	if err != nil || current != expected {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, expected, current)
	}
	base, err := r.run("merge-base", string(expected), string(candidate))
	if err != nil || strings.TrimSpace(base) != string(expected) {
		return "", kernel.Fail(kernel.ErrNonFastForward, "commit %s is not a descendant of %s", candidate, expected)
	}
	if _, err := r.run("checkout", branch); err != nil {
		return "", err
	}
	if _, err := r.run("reset", "--hard", string(candidate)); err != nil {
		return "", err
	}
	return candidate, nil
}

func (r *DoltRepository) Archived() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.archivedLocked()
}

func (r *DoltRepository) Archive() error {
	if r.Archived() {
		return nil
	}
	head, err := r.Head(snapshot.DefaultRef)
	if err != nil {
		return err
	}
	if err := r.CreateRef("refs/kc/archived", head); err != nil {
		return err
	}
	r.lock.Lock()
	r.archived = true
	r.lock.Unlock()
	return nil
}

func (r *DoltRepository) archivedLocked() bool {
	if r.archived {
		return true
	}
	rows, err := r.query("SELECT hash FROM dolt_branches WHERE name=" + sqlString("kc-archived"))
	if err == nil && len(rows) == 1 {
		r.archived = true
	}
	return r.archived
}

package filegit

import (
	"os"
	"path/filepath"

	"kc/internal/gitdir"
	"kc/internal/treepath"
	"kc/kernel"
	"kc/snapshot"
)

// TreeStore (snapshot.TreeStore): literal path read/write at a
// commit, no frontmatter, no object_id. Sibling to the Knowledge methods in
// filegit.go / commit.go, not a replacement for them.
var _ snapshot.TreeStore = (*FileGitRepository)(nil)

func (r *FileGitRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	clean, err := treepath.Clean(path)
	if err != nil {
		return nil, err
	}
	// git show <rev>:<path> does not fail for a directory path — it prints
	// that tree's listing as if it were file content — so ReadFile must
	// check the object type itself before trusting ShowRaw's bytes.
	kind, ok := r.dir.ObjectType(string(commit), clean)
	if !ok || kind != "blob" {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
	}
	raw, err := r.dir.ShowRaw(string(commit), clean)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is missing at commit %s", path, commit)
	}
	return raw, nil
}

func (r *FileGitRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	paths, err := r.dir.Paths(string(commit))
	if err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	return paths, nil
}

// ApplyTreeCommit applies literal bytes at literal paths with ref CAS. It has
// no Address, schema_ref, or other Knowledge semantics.
func (r *FileGitRepository) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	if cs.TargetRepository != r.repositoryID {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.repositoryID)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	if len(cs.Changes) == 0 {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "raw changeset has no changes")
	}
	targetRef := cs.TargetRef
	isHead := targetRef == "" || targetRef == "HEAD"
	var current kernel.CommitID
	if isHead {
		h, err := r.Head("")
		if err != nil {
			return "", err
		}
		current = h
	} else if c, ok := r.GetRef(targetRef); ok {
		current = c
	}
	if current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, cs.ExpectedTargetCommit, current)
	}
	checkedOut := r.dir.CheckedOutRef()
	needsSwitch := !isHead && checkedOut != targetRef
	var restore string
	if needsSwitch {
		if checkedOut != "" {
			restore = checkedOut
		} else {
			h, _ := r.Head("")
			restore = string(h)
		}
		if err := r.dir.Checkout(gitdir.BranchName(targetRef)); err != nil {
			return "", err
		}
	}
	restoreCheckout := func() {
		if restore != "" {
			_ = r.dir.Checkout(gitdir.BranchName(restore))
		}
	}
	if r.dir.Dirty() {
		restoreCheckout()
		return "", kernel.Fail(kernel.ErrPreconditionFailed, "working tree must be clean before a raw commit")
	}
	for _, ch := range cs.Changes {
		clean, err := treepath.Clean(ch.Path)
		if err != nil {
			restoreCheckout()
			return "", err
		}
		full := filepath.Join(r.rootDir, clean)
		if ch.Remove {
			_ = os.Remove(full)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			restoreCheckout()
			return "", err
		}
		if err := os.WriteFile(full, ch.Content, 0o644); err != nil {
			restoreCheckout()
			return "", err
		}
	}
	if err := r.dir.StageAll(); err != nil {
		restoreCheckout()
		return "", err
	}
	newCommit, err := r.dir.Commit(gitdir.Signature{
		Author: cs.Author, Message: cs.Message, RequestID: cs.RequestID, RuleID: cs.RuleID,
	}, true)
	if err != nil {
		restoreCheckout()
		return "", err
	}
	restoreCheckout()
	return kernel.CommitID(newCommit), nil
}

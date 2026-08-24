package local

import (
	"os"
	"path/filepath"

	"kc/internal/gitdir"
	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

func (r *FileGitRepository) ApplyCommit(cs repository.CommitChangeSet) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	if err := kernel.ValidateProvenance(cs.Provenance); err != nil {
		return "", err
	}
	if cs.TargetRepository != r.repositoryID {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.repositoryID)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	targetRef := cs.TargetRef
	isHead := targetRef == "HEAD"
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
		return "", kernel.Fail(kernel.ErrPreconditionFailed, "working tree must be clean before protocol COMMIT")
	}
	idx, err := r.scan()
	if err != nil {
		restoreCheckout()
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, op := range cs.Operations {
		if err := repofile.Apply(idx, op, cs.Provenance, toWrite, toDelete); err != nil {
			restoreCheckout()
			return "", err
		}
	}
	for p := range toDelete {
		if _, ok := toWrite[p]; !ok {
			_ = os.Remove(filepath.Join(r.rootDir, p))
		}
	}
	for p, content := range toWrite {
		full := filepath.Join(r.rootDir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			restoreCheckout()
			return "", err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			restoreCheckout()
			return "", err
		}
	}
	if err := r.dir.StageAll(); err != nil {
		restoreCheckout()
		return "", err
	}
	newCommit, err := r.dir.Commit(commitSignature(cs), true)
	if err != nil {
		restoreCheckout()
		return "", err
	}
	restoreCheckout()
	return kernel.CommitID(newCommit), nil
}

// commitSignature is the shared kc commit convention (author fallback plus
// Request-Id / Rule-Id trailers), identical on every Snapshot backend.
func commitSignature(cs repository.CommitChangeSet) gitdir.Signature {
	return gitdir.Signature{
		Author:    cs.Author,
		Message:   cs.Message,
		RequestID: cs.RequestID,
		RuleID:    cs.RuleID,
	}
}

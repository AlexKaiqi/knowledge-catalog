package local

import (
	"os"
	"path/filepath"
	"strings"

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
		return "", kernel.Fail(kernel.ErrPreconditionFailed, "baseCommit must equal expectedTargetCommit")
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
		return "", kernel.Fail(kernel.ErrNonFastForward, "expected %s but ref is %s", cs.ExpectedTargetCommit, current)
	}
	checkedOut, _ := git(r.rootDir, "symbolic-ref", "-q", "HEAD")
	needsSwitch := !isHead && checkedOut != targetRef
	var restore string
	if needsSwitch {
		if checkedOut != "" {
			restore = checkedOut
		} else {
			h, _ := r.Head("")
			restore = string(h)
		}
		if _, err := git(r.rootDir, "checkout", "-q", checkoutName(targetRef)); err != nil {
			return "", err
		}
	}
	restoreCheckout := func() {
		if restore != "" {
			_, _ = git(r.rootDir, "checkout", "-q", checkoutName(restore))
		}
	}
	dirty, _ := git(r.rootDir, "status", "--porcelain")
	if dirty != "" {
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
	if _, err := git(r.rootDir, "add", "-A"); err != nil {
		restoreCheckout()
		return "", err
	}
	name, email, msg := gitCommitIdentity(cs)
	if _, err := git(r.rootDir, "-c", "user.name="+name, "-c", "user.email="+email, "commit", "--allow-empty", "-q", "-m", msg); err != nil {
		restoreCheckout()
		return "", err
	}
	newCommit, err := r.Head("")
	if restore != "" {
		_, _ = git(r.rootDir, "checkout", "-q", checkoutName(restore))
	}
	return newCommit, err
}

func gitCommitIdentity(cs repository.CommitChangeSet) (name, email, message string) {
	name = strings.TrimSpace(strings.ReplaceAll(cs.Author, "\n", " "))
	if name == "" {
		name = "knowledge-catalog"
	}
	if len(name) > 128 {
		name = name[:128]
	}
	email = "kc@local"
	message = strings.TrimSpace(cs.Message)
	if message == "" {
		message = "commit"
	}
	var trailers []string
	if id := strings.TrimSpace(cs.RequestID); id != "" {
		trailers = append(trailers, "Request-Id: "+id)
	}
	if id := strings.TrimSpace(cs.RuleID); id != "" {
		trailers = append(trailers, "Rule-Id: "+id)
	}
	if len(trailers) > 0 {
		message += "\n\n" + strings.Join(trailers, "\n")
	}
	return name, email, message
}

// CommitWorktree stages the working tree and commits it. Used by Catalog
// registry YAML files, which are not knowledge objects.
func (r *FileGitRepository) CommitWorktree(expected kernel.CommitID, message, author, requestID, ruleID string) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	current, err := r.Head("refs/heads/main")
	if err != nil {
		return "", err
	}
	if current != expected {
		return "", kernel.Fail(kernel.ErrNonFastForward, "expected %s but ref is %s", expected, current)
	}
	if _, err := git(r.rootDir, "checkout", "-q", "main"); err != nil {
		return "", err
	}
	if _, err := git(r.rootDir, "add", "-A"); err != nil {
		return "", err
	}
	dirty, _ := git(r.rootDir, "status", "--porcelain")
	if dirty == "" {
		return current, nil
	}
	name, email, msg := gitCommitIdentity(repository.CommitChangeSet{
		Message: message, Author: author, RequestID: requestID, RuleID: ruleID,
	})
	if _, err := git(r.rootDir, "-c", "user.name="+name, "-c", "user.email="+email, "commit", "-q", "-m", msg); err != nil {
		return "", err
	}
	return r.Head("")
}


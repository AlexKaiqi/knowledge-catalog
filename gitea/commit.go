package gitea

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kc/internal/gitdir"
	"kc/internal/repofile"
	"kc/kernel"
	"kc/repository"
)

func (r *Repository) ApplyCommit(cs repository.CommitChangeSet) (kernel.CommitID, error) {
	if err := r.denyIfArchived(); err != nil {
		return "", err
	}
	if err := kernel.ValidateProvenance(cs.Provenance); err != nil {
		return "", err
	}
	if cs.TargetRepository != r.id {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.id)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	targetRef := cs.TargetRef
	if targetRef == "" || targetRef == "HEAD" {
		targetRef = "refs/heads/" + r.branch
	}
	current, ok := r.GetRef(targetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrVersionUnresolved, "ref %s does not exist", targetRef)
	}
	if current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", targetRef, cs.ExpectedTargetCommit, current)
	}
	idx, blobs, err := r.scanAt(cs.ExpectedTargetCommit)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, op := range cs.Operations {
		if err := repofile.Apply(idx, op, cs.Provenance, toWrite, toDelete); err != nil {
			return "", err
		}
	}
	files := changeOps(toWrite, toDelete, blobs)
	if len(files) == 0 {
		return current, nil
	}
	name, email, msg := commitSignature(cs).Format()
	wip := r.newWipName()
	if err := r.createBranch(wip, cs.ExpectedTargetCommit); err != nil {
		return "", err
	}
	defer r.deleteBranch(wip)
	sha, err := r.changeFiles(wip, files, name, email, msg)
	if err != nil {
		return "", mapWriteErr(err, current)
	}
	if err := r.updateBranch(branchName(targetRef, r.branch), sha, cs.ExpectedTargetCommit); err != nil {
		return "", mapWriteErr(err, current)
	}
	r.invalidate()
	return sha, nil
}

func (r *Repository) newWipName() string {
	r.mu.Lock()
	r.wip++
	n := r.wip
	r.mu.Unlock()
	return fmt.Sprintf("kc-wip/%d-%d", time.Now().UnixNano(), n)
}

func (r *Repository) changeFiles(branch string, files []changeFileOp, name, email, msg string) (kernel.CommitID, error) {
	body := changeFilesBody{
		Message:   msg,
		Branch:    branch,
		Author:    gitIdentity{Name: name, Email: email},
		Committer: gitIdentity{Name: name, Email: email},
		Files:     files,
	}
	var out filesResponse
	if _, _, err := r.cli.do(http.MethodPost, r.ep.repoPath("contents"), body, &out); err != nil {
		return "", err
	}
	sha := out.sha()
	if sha == "" {
		if head, ok := r.lookupBranch(branch); ok {
			return head, nil
		}
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea commit sha missing")
	}
	return kernel.CommitID(sha), nil
}

func changeOps(toWrite map[string]string, toDelete map[string]struct{}, blobs map[string]string) []changeFileOp {
	var files []changeFileOp
	for path, content := range toWrite {
		op := changeFileOp{
			Path:    path,
			Content: base64.StdEncoding.EncodeToString([]byte(content)),
		}
		if sha, ok := blobs[path]; ok {
			op.Operation = "update"
			op.SHA = sha
		} else {
			op.Operation = "create"
		}
		files = append(files, op)
	}
	for path := range toDelete {
		if _, written := toWrite[path]; written {
			continue
		}
		files = append(files, changeFileOp{Operation: "delete", Path: path, SHA: blobs[path]})
	}
	return files
}

func branchName(ref, fallback string) string {
	if ref == "" || ref == "HEAD" {
		return fallback
	}
	return strings.TrimPrefix(ref, "refs/heads/")
}

// commitSignature keeps Gitea commits byte-identical to FileGit commits, so
// `kc audit` reads the same author and Request-Id / Rule-Id trailers on either
// backend. The convention itself lives in internal/gitdir.
func commitSignature(cs repository.CommitChangeSet) gitdir.Signature {
	return gitdir.Signature{
		Author:    cs.Author,
		Message:   cs.Message,
		RequestID: cs.RequestID,
		RuleID:    cs.RuleID,
	}
}

func mapWriteErr(err error, current kernel.CommitID) error {
	if err == nil {
		return nil
	}
	switch statusOf(err) {
	case http.StatusLocked:
		return kernel.Fail(kernel.ErrRepositoryArchived, "%s", err.Error())
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return kernel.Fail(kernel.ErrNonFastForward, "ref update rejected: expected commit %s", current)
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "archived") {
		return kernel.Fail(kernel.ErrRepositoryArchived, "%s", err.Error())
	}
	if strings.Contains(msg, "409") || strings.Contains(msg, "422") || strings.Contains(msg, "sha") {
		return kernel.Fail(kernel.ErrNonFastForward, "ref update rejected: expected commit %s", current)
	}
	return err
}

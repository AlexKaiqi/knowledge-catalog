package gitea

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kc/kernel"
)

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

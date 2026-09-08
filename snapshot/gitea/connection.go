package gitea

import (
	"net/http"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// OpenConnection attaches an existing authority using only the explicitly
// supplied credential. Numeric provider identity prevents URL reuse from
// silently redirecting an established connection to another repository.
func OpenConnection(id kernel.RepositoryID, dsn, credential string, expected int64) (*Repository, int64, error) {
	if strings.TrimSpace(credential) == "" {
		return nil, 0, kernel.Fail(kernel.ErrUnauthenticated, "connection credential is required")
	}
	r, err := newRepository(id, dsn, credential)
	if err != nil {
		return nil, 0, err
	}
	var info repoInfo
	if _, _, err := r.cli.do(http.MethodGet, r.ep.repoPath(""), nil, &info); err != nil {
		return nil, 0, err
	}
	if info.ID <= 0 || (expected != 0 && info.ID != expected) {
		return nil, 0, kernel.Fail(kernel.ErrPreconditionFailed, "connected Gitea authority identity changed")
	}
	if info.Empty {
		return nil, 0, kernel.Fail(kernel.ErrVersionUnresolved, "connected Gitea authority has no published commit")
	}
	if info.DefaultBranch != "" {
		r.branch = info.DefaultBranch
	}
	probe := *r.cli
	r.cli.guard = func() error {
		var current repoInfo
		if _, _, err := probe.do(http.MethodGet, r.ep.repoPath(""), nil, &current); err != nil {
			return err
		}
		if current.ID != info.ID {
			return kernel.Fail(kernel.ErrPreconditionFailed, "connected Gitea authority identity changed")
		}
		return nil
	}
	head, err := r.Head(snapshot.DefaultRef)
	if err != nil {
		return nil, 0, err
	}
	if !r.HasCommit(head) {
		return nil, 0, kernel.Fail(kernel.ErrVersionUnresolved, "connected published commit is unavailable")
	}
	return r, info.ID, nil
}

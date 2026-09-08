package gitea

import (
	"net/http"
	"net/url"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

func managedDescription(id kernel.RepositoryID, allocation string) string {
	return "kc-managed:" + allocation + ":" + string(id)
}

func (r *Repository) verifyManaged(info repoInfo, allocation string, backendID int64) error {
	if allocation == "" || info.ID <= 0 || info.Description != managedDescription(r.id, allocation) || (backendID != 0 && info.ID != backendID) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "Gitea repository is not the expected managed allocation")
	}
	return nil
}

func (r *Repository) guardManaged(allocation string, backendID int64) {
	probe := *r.cli
	probe.guard = nil
	r.cli.guard = func() error {
		var info repoInfo
		if _, _, err := probe.do(http.MethodGet, r.ep.repoPath(""), nil, &info); err != nil {
			return err
		}
		return r.verifyManaged(info, allocation, backendID)
	}
}

// CreateManaged creates or resumes only the allocation owned by this command.
// The allocation must have been durably reserved before calling this method.
// Repository metadata carries the ownership token in the same create request,
// allowing an accepted request with a lost response to be recognized safely.
func CreateManaged(id kernel.RepositoryID, dsn, token, allocation string) (*Repository, int64, error) {
	return createManaged(id, dsn, token, allocation, false)
}

// CreateManagedForUser provisions beneath a previously verified or allocated
// user account. The admin API does not mistake another user for an organization.
func CreateManagedForUser(id kernel.RepositoryID, dsn, token, allocation string) (*Repository, int64, error) {
	return createManaged(id, dsn, token, allocation, true)
}

func createManaged(id kernel.RepositoryID, dsn, token, allocation string, userOwned bool) (*Repository, int64, error) {
	if strings.TrimSpace(allocation) == "" {
		return nil, 0, kernel.Fail(kernel.ErrUsageInvalid, "managed allocation identity is required")
	}
	r, err := newRepository(id, dsn, token)
	if err != nil {
		return nil, 0, err
	}
	var info repoInfo
	status, _, probeErr := r.cli.do(http.MethodGet, r.ep.repoPath(""), nil, &info)
	if status == http.StatusNotFound {
		var me userInfo
		if _, _, err := r.cli.do(http.MethodGet, "/user", nil, &me); err != nil {
			return nil, 0, err
		}
		path := "/user/repos"
		if me.Login != r.ep.Owner {
			path = "/orgs/" + url.PathEscape(r.ep.Owner) + "/repos"
			if userOwned {
				path = "/admin/users/" + url.PathEscape(r.ep.Owner) + "/repos"
			}
		}
		body := createRepoBody{Name: r.ep.Name, Private: true, AutoInit: true, DefaultBranch: defaultBranch, Description: managedDescription(id, allocation)}
		if _, _, err := r.cli.do(http.MethodPost, path, body, &info); err != nil {
			// A transport failure is ambiguous. Never allocate another name or
			// accept a conflict blindly: inspect the same resource's ownership.
			if _, _, readErr := r.cli.do(http.MethodGet, r.ep.repoPath(""), nil, &info); readErr != nil {
				return nil, 0, err
			}
		}
	} else if probeErr != nil {
		return nil, 0, probeErr
	}
	if err := r.verifyManaged(info, allocation, 0); err != nil {
		return nil, 0, err
	}
	r.guardManaged(allocation, info.ID)
	if info.DefaultBranch != "" {
		r.branch = info.DefaultBranch
	}
	needsInitialization := info.Empty
	if !needsInitialization {
		var branch branchInfo
		status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("branches/"+encodeRefPath(r.branch)), nil, &branch)
		if status == http.StatusNotFound {
			needsInitialization = true
		} else if err != nil {
			return nil, 0, err
		} else if branch.commitID() == "" {
			return nil, 0, kernel.Fail(kernel.ErrVersionUnresolved, "managed Gitea branch has no published commit")
		}
	}
	if needsInitialization {
		if err := r.initMain(); err != nil {
			return nil, 0, err
		}
	}
	if _, err := r.Head(snapshot.DefaultRef); err != nil {
		return nil, 0, err
	}
	return r, info.ID, nil
}

// OpenManaged verifies allocation and immutable backend identity before a
// read-only open. Missing or replaced repositories are never recreated.
func OpenManaged(id kernel.RepositoryID, dsn, token, allocation string, backendID int64) (*Repository, error) {
	if backendID <= 0 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed Gitea backend identity is missing")
	}
	r, err := newRepository(id, dsn, token)
	if err != nil {
		return nil, err
	}
	var info repoInfo
	if _, _, err := r.cli.do(http.MethodGet, r.ep.repoPath(""), nil, &info); err != nil {
		return nil, err
	}
	if err := r.verifyManaged(info, allocation, backendID); err != nil {
		return nil, err
	}
	opened, err := OpenExisting(id, dsn, token)
	if err != nil {
		return nil, err
	}
	opened.guardManaged(allocation, backendID)
	return opened, nil
}

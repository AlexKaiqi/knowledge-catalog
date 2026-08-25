package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"kc/kernel"
)

// ResolvedWorkspace is one command pin: snapshot {repo → commit}.
// Taken at ResolveWorkspace (live selectors, or an overlay for preview).
// Not a registry object. Catalog stops here: no object_id, no event payload.

type ResolvedWorkspace struct {
	WorkspaceID  string                                  `json:"workspaceId"`
	Revision     int                                     `json:"revision"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	// PinID is the content-address of this pin: workspace id, path layout,
	// {repo→commit}. Revision is a recipe counter and does
	// not participate. Re-export and pass --pin to replay; replay still
	// evaluates allow per member (docs/COMPOSITION.md).
	PinID string `json:"pinId,omitempty"`
}

type WorkspaceIssue struct {
	Repository kernel.RepositoryID `json:"repository"`
	Code       kernel.ErrorCode    `json:"code"`
	Message    string              `json:"message"`
}

type WorkspaceCheck struct {
	WorkspaceID string           `json:"workspaceId"`
	Outcome     string           `json:"outcome"`
	Issues      []WorkspaceIssue `json:"issues"`
}

// HashResolved is the content-address of everything that determines what a
// consumer would read at this pin. Revision is excluded: two recipe edits
// that leave membership, layout and commits unchanged are the same pin.
func HashResolved(workspaceID string, sources []WorkspaceSource, repos map[kernel.RepositoryID]kernel.CommitID) string {
	keys := make([]string, 0, len(repos))
	for k := range repos {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	byRepo := map[kernel.RepositoryID]WorkspaceSource{}
	for _, src := range sources {
		byRepo[src.Repository] = src
	}
	s := workspaceID
	for _, k := range keys {
		id := kernel.RepositoryID(k)
		s += "," + k + "=" + string(repos[id])
		src, ok := byRepo[id]
		if !ok {
			continue
		}
		s += "@" + mountHashToken(src.Path)
		if src.SubPath != "" {
			s += "#" + strings.Trim(src.SubPath, "/")
		}
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// mountHashToken distinguishes "not a mount" from "mounted at root": both
// are legal WorkspaceSource.Path values but they are not the same pin.
func mountHashToken(path *string) string {
	if path == nil {
		return "-"
	}
	return normalizeMountPath(*path)
}

func (c *Catalog) ResolveWorkspace(workspaceID string) (ResolvedWorkspace, error) {
	return c.ResolveWorkspaceOverlay(workspaceID, nil)
}

func (c *Catalog) ResolveWorkspaceOverlay(workspaceID string, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedWorkspace, error) {
	def, err := c.Workspace(workspaceID)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	return c.ResolveDefinitionOverlay(def, overlay)
}

func (c *Catalog) ResolveDefinition(def WorkspaceDefinition) (ResolvedWorkspace, error) {
	return c.ResolveDefinitionOverlay(def, nil)
}

func (c *Catalog) ResolveDefinitionOverlay(def WorkspaceDefinition, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedWorkspace, error) {
	if def.Retired {
		return ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is retired", def.WorkspaceID)
	}
	if len(def.Sources) == 0 {
		return ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "a workspace must contain at least one repository")
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		if err := c.requireRepository(src.Repository); err != nil {
			return ResolvedWorkspace{}, err
		}
		if _, err := c.store.Require(src.Repository, kernel.ErrWorkspaceInvalid); err != nil {
			return ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace recipe names unknown repository %s", src.Repository)
		}
		repo, _ := c.store.Get(src.Repository)
		if repo.Archived() {
			return ResolvedWorkspace{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", src.Repository)
		}
		commit, ok := repo.GetRef(src.Selector)
		if !ok {
			return ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s has no ref %s", src.Repository, src.Selector)
		}
		if src.BaseRev != "" {
			want := kernel.CommitID(src.BaseRev)
			if !repo.HasCommit(want) {
				return ResolvedWorkspace{}, kernel.Fail(kernel.ErrVersionUnresolved, "baseRev %s does not exist in %s", src.BaseRev, src.Repository)
			}
			if commit != want {
				return ResolvedWorkspace{}, kernel.Fail(kernel.ErrNonFastForward,
					"repository %s selector %s is at %s, recipe baseRev is %s", src.Repository, src.Selector, commit, src.BaseRev)
			}
		}
		if overlayCommit, hit := overlay[src.Repository]; hit {
			if !repo.HasCommit(overlayCommit) {
				return ResolvedWorkspace{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", overlayCommit, src.Repository)
			}
			commit = overlayCommit
		}
		repositories[src.Repository] = commit
	}
	for repositoryID := range overlay {
		if _, ok := repositories[repositoryID]; !ok {
			return ResolvedWorkspace{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s is not in the workspace", repositoryID)
		}
	}
	return ResolvedWorkspace{
		WorkspaceID:  def.WorkspaceID,
		Revision:     def.Revision,
		Repositories: repositories,
		PinID:        HashResolved(def.WorkspaceID, def.Sources, repositories),
	}, nil
}

func (c *Catalog) CheckResolved(resolved ResolvedWorkspace) WorkspaceCheck {
	issues := []WorkspaceIssue{}
	for repositoryID, commit := range resolved.Repositories {
		repo, ok := c.store.Get(repositoryID)
		if !ok {
			issues = append(issues, WorkspaceIssue{
				Repository: repositoryID,
				Code:       kernel.ErrUsageInvalid,
				Message:    "repository " + string(repositoryID) + " is not mounted",
			})
			continue
		}
		if !repo.HasCommit(commit) {
			issues = append(issues, WorkspaceIssue{
				Repository: repositoryID,
				Code:       kernel.ErrVersionUnresolved,
				Message:    "commit " + string(commit) + " does not exist in " + string(repositoryID),
			})
		}
	}
	outcome := "PASSED"
	if len(issues) > 0 {
		outcome = "FAILED"
	}
	return WorkspaceCheck{WorkspaceID: resolved.WorkspaceID, Outcome: outcome, Issues: issues}
}

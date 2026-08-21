package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"kc/kernel"
)

// ResolvedView is one OpenView: the recipe plus {repo → commit} taken at open
// (live selectors, or an overlay for preview). Not a registry object.

type ResolvedView struct {
	ViewID       string                                  `json:"viewId"`
	Revision     int                                     `json:"revision"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
}

// ViewReadVersion is the consumer read basis for one Serving session.
type ViewReadVersion struct {
	Resolved                 ResolvedView     `json:"resolved"`
	AppendCuts               map[string]string `json:"appendCuts,omitempty"`
	AuthorizationDecisionRef string            `json:"authorizationDecisionRef,omitempty"`
}

type FederatedValue struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	ObjectID   kernel.ObjectID     `json:"objectId"`
	Value      any                 `json:"value"`
}

type ViewIssue struct {
	Repository kernel.RepositoryID `json:"repository"`
	Code       kernel.ErrorCode    `json:"code"`
	Message    string              `json:"message"`
}

type ViewCheck struct {
	ViewID  string      `json:"viewId"`
	Outcome string      `json:"outcome"`
	Issues  []ViewIssue `json:"issues"`
}

func HashResolved(viewID string, repos map[kernel.RepositoryID]kernel.CommitID) string {
	keys := make([]string, 0, len(repos))
	for k := range repos {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	s := viewID
	for _, k := range keys {
		s += "," + k + "=" + string(repos[kernel.RepositoryID(k)])
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (c *Catalog) ResolveView(viewID string) (ResolvedView, error) {
	return c.ResolveViewOverlay(viewID, nil)
}

func (c *Catalog) ResolveViewOverlay(viewID string, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedView, error) {
	def, err := c.View(viewID)
	if err != nil {
		return ResolvedView{}, err
	}
	if def.Retired {
		return ResolvedView{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "view %s is retired", viewID)
	}
	if len(def.Sources) == 0 {
		return ResolvedView{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "a view must contain at least one repository")
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		if err := c.requireRepository(src.Repository); err != nil {
			return ResolvedView{}, err
		}
		if _, err := c.store.Require(src.Repository, kernel.ErrViewGenerationInvalid); err != nil {
			return ResolvedView{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "unknown repository %s", src.Repository)
		}
		repo, _ := c.store.Get(src.Repository)
		if repo.Archived() {
			return ResolvedView{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", src.Repository)
		}
		commit, ok := repo.GetRef(src.Selector)
		if !ok {
			return ResolvedView{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "selector %s is unresolved in %s", src.Selector, src.Repository)
		}
		if overlayCommit, hit := overlay[src.Repository]; hit {
			if !repo.HasCommit(overlayCommit) {
				return ResolvedView{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s is unresolved in %s", overlayCommit, src.Repository)
			}
			commit = overlayCommit
		}
		repositories[src.Repository] = commit
	}
	for repositoryID := range overlay {
		if _, ok := repositories[repositoryID]; !ok {
			return ResolvedView{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "repository %s is not in the view", repositoryID)
		}
	}
	return ResolvedView{ViewID: def.ViewID, Revision: def.Revision, Repositories: repositories}, nil
}

func (c *Catalog) FederatedRead(viewID string, objectID kernel.ObjectID) ([]FederatedValue, error) {
	serving, err := c.OpenView(viewID)
	if err != nil {
		return nil, err
	}
	return serving.Read(objectID, nil)
}

func (c *Catalog) CheckResolved(resolved ResolvedView) ViewCheck {
	issues := []ViewIssue{}
	for repositoryID, commit := range resolved.Repositories {
		repo, ok := c.store.Get(repositoryID)
		if !ok {
			issues = append(issues, ViewIssue{
				Repository: repositoryID,
				Code:       kernel.ErrTemporaryUnavailable,
				Message:    string(repositoryID) + " is not mounted",
			})
			continue
		}
		if !repo.HasCommit(commit) {
			issues = append(issues, ViewIssue{
				Repository: repositoryID,
				Code:       kernel.ErrVersionUnresolved,
				Message:    "commit " + string(commit) + " is unresolved in " + string(repositoryID),
			})
		}
	}
	outcome := "PASSED"
	if len(issues) > 0 {
		outcome = "FAILED"
	}
	return ViewCheck{ViewID: resolved.ViewID, Outcome: outcome, Issues: issues}
}

package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"kc/kernel"
)

// ResolvedDataset is one command pin: the published file list at a Dataset
// version (and the {Repository → commit} map that list implies). Taken at
// ResolveDataset from frozen source commits, or an overlay for preview.
// Not a registry object. service stops here: no object_id, no event payload.

type ResolvedDataset struct {
	SetID    string `json:"setId"`
	Revision int    `json:"revision"`
	// Ref is the published Dataset version label: "vN" for this revision.
	// latest is service.Set (the current maximum revision). One request
	// resolves this once (V-01); storage stays the integer revision.
	Ref          string                                  `json:"ref,omitempty"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	Items        []DatasetItem                           `json:"items,omitempty"`
	// PinID is the content-address of this pin: workspace id, path layout,
	// {Repository→commit}. Revision is a recipe counter and does
	// not participate. Re-export and pass --pin to replay; replay still
	// evaluates allow per member (doc/design/dataset.md).
	PinID string `json:"pinId,omitempty"`
}

type DatasetIssue struct {
	Repository kernel.RepositoryID `json:"repository"`
	Code       kernel.ErrorCode    `json:"code"`
	Message    string              `json:"message"`
}

type DatasetCheck struct {
	SetID   string         `json:"setId"`
	Outcome string         `json:"outcome"`
	Issues  []DatasetIssue `json:"issues"`
}

// HashResolved is the content-address of everything that determines what a
// consumer would read at this pin. Revision is excluded: two recipe edits
// that leave membership, layout and commits unchanged are the same pin.
func HashResolved(setID string, sources []DatasetSource, repos map[kernel.RepositoryID]kernel.CommitID) string {
	keys := make([]string, 0, len(repos))
	for k := range repos {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	byRepo := map[kernel.RepositoryID][]DatasetSource{}
	for _, src := range sources {
		byRepo[src.Repository] = append(byRepo[src.Repository], src)
	}
	s := setID
	for _, k := range keys {
		id := kernel.RepositoryID(k)
		s += "," + k + "=" + string(repos[id])
		entries, ok := byRepo[id]
		if !ok {
			continue
		}
		tokens := make([]string, 0, len(entries))
		for _, src := range entries {
			if src.IsFileEntry() {
				tokens = append(tokens, "=f:"+strings.Trim(src.File, "/")+"#"+normalizeMountPath(src.Target))
				continue
			}
			token := "@" + mountHashToken(src.Path)
			if src.SubPath != "" {
				token += "#" + strings.Trim(src.SubPath, "/")
			}
			tokens = append(tokens, token)
		}
		sort.Strings(tokens)
		// One plain mount keeps the historical single-token PinID; any file
		// entry changes what a consumer reads, so it participates in the pin.
		if len(tokens) == 1 && !strings.HasPrefix(tokens[0], "=f:") {
			s += tokens[0]
		} else {
			s += "\x00" + strings.Join(tokens, "\x00")
		}
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// mountHashToken distinguishes "not a mount" from "mounted at root": both
// are legal DatasetSource.Path values but they are not the same pin.
func mountHashToken(path *string) string {
	if path == nil {
		return "-"
	}
	return normalizeMountPath(*path)
}

func (c *service) ResolveDataset(setID string) (ResolvedDataset, error) {
	return c.ResolveDatasetOverlay(setID, nil)
}

func (c *service) ResolveDatasetOverlay(setID string, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedDataset, error) {
	def, err := c.Set(setID)
	if err != nil {
		return ResolvedDataset{}, err
	}
	return c.ResolveDefinitionOverlay(def, overlay)
}

func (c *service) ResolveDefinition(def Dataset) (ResolvedDataset, error) {
	return c.ResolveDefinitionOverlay(def, nil)
}

func (c *service) ResolveDefinitionOverlay(def Dataset, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedDataset, error) {
	if def.Retired {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "workspace %s is retired", def.SetID)
	}
	if len(def.Sources) == 0 {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "a workspace must contain at least one repository")
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return ResolvedDataset{}, err
	}
	if err := validateSourceCoordinates(def.Sources); err != nil {
		return ResolvedDataset{}, err
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		if _, resolved := repositories[src.Repository]; resolved {
			continue
		}
		if err := c.requireRepository(src.Repository); err != nil {
			return ResolvedDataset{}, err
		}
		if _, err := c.store.Require(src.Repository, kernel.ErrDatasetInvalid); err != nil {
			return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "workspace recipe names unknown repository %s", src.Repository)
		}
		repo, _ := c.store.Get(src.Repository)
		if repo.Archived() {
			return ResolvedDataset{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", src.Repository)
		}
		var commit kernel.CommitID
		if overlayCommit, hit := overlay[src.Repository]; hit {
			if !repo.HasCommit(overlayCommit) {
				return ResolvedDataset{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", overlayCommit, src.Repository)
			}
			commit = overlayCommit
		} else if src.Commit != "" {
			if !repo.HasCommit(src.Commit) {
				return ResolvedDataset{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", src.Commit, src.Repository)
			}
			commit = src.Commit
		} else {
			resolved, ok := repo.GetRef(src.Selector)
			if !ok {
				return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "repository %s has no ref %s", src.Repository, src.Selector)
			}
			if src.BaseRev != "" {
				want := kernel.CommitID(src.BaseRev)
				if !repo.HasCommit(want) {
					return ResolvedDataset{}, kernel.Fail(kernel.ErrVersionUnresolved, "baseRev %s does not exist in %s", src.BaseRev, src.Repository)
				}
				if resolved != want {
					return ResolvedDataset{}, kernel.Fail(kernel.ErrNonFastForward,
						"repository %s selector %s is at %s, recipe baseRev is %s", src.Repository, src.Selector, resolved, src.BaseRev)
				}
			}
			commit = resolved
		}
		repositories[src.Repository] = commit
	}
	for repositoryID := range overlay {
		if _, ok := repositories[repositoryID]; !ok {
			return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not in the workspace", repositoryID)
		}
	}
	items, err := datasetItemsFromSources(def.Sources, repositories)
	if err != nil {
		return ResolvedDataset{}, err
	}
	return ResolvedDataset{
		SetID:        def.SetID,
		Revision:     def.Revision,
		Ref:          DatasetVersionRef(def.Revision),
		Repositories: repositories,
		Items:        items,
		PinID:        HashResolved(def.SetID, def.Sources, repositories),
	}, nil
}

// DatasetVersionRef is the public vN label for a stored integer revision.
func DatasetVersionRef(revision int) string {
	if revision <= 0 {
		return ""
	}
	return "v" + strconv.Itoa(revision)
}

func (c *service) CheckResolved(resolved ResolvedDataset) DatasetCheck {
	issues := []DatasetIssue{}
	for repositoryID, commit := range resolved.Repositories {
		issues = append(issues, c.checkResolvedItems(resolved, repositoryID, commit)...)
	}
	return datasetCheck(resolved.SetID, issues)
}

// CheckResolvedRepository validates the one member a path-routed operation
// will actually read. The caller must first validate the pin's Workspace
// identity, membership and PinID against the effective definition. This keeps
// one VFS file read proportional to its owning Repository instead of probing
// every unrelated member in the Workspace.
func (c *service) CheckResolvedRepository(resolved ResolvedDataset, repositoryID kernel.RepositoryID) DatasetCheck {
	commit, ok := resolved.Repositories[repositoryID]
	if !ok {
		return datasetCheck(resolved.SetID, []DatasetIssue{{
			Repository: repositoryID,
			Code:       kernel.ErrDatasetInvalid,
			Message:    "resolved pin has no commit for repository " + string(repositoryID),
		}})
	}
	return datasetCheck(resolved.SetID, c.checkResolvedRepository(repositoryID, commit))
}

func (c *service) checkResolvedItems(resolved ResolvedDataset, repositoryID kernel.RepositoryID, commit kernel.CommitID) []DatasetIssue {
	issues := c.checkResolvedRepository(repositoryID, commit)
	if len(issues) != 0 {
		return issues
	}
	for _, item := range resolved.Items {
		if item.Repository != repositoryID || item.Kind != DatasetItemFile {
			continue
		}
		if item.Commit != commit {
			issues = append(issues, DatasetIssue{Repository: repositoryID, Code: kernel.ErrDatasetInvalid, Message: "selected file disagrees with resolved repository commit"})
			continue
		}
		if err := c.validateSelectedFile(item); err != nil {
			issues = append(issues, DatasetIssue{Repository: repositoryID, Code: kernel.CodeOf(err), Message: err.Error()})
		}
	}
	return issues
}

func (c *service) checkResolvedRepository(repositoryID kernel.RepositoryID, commit kernel.CommitID) []DatasetIssue {
	if !c.HasRepository(repositoryID) {
		return []DatasetIssue{{Repository: repositoryID, Code: kernel.ErrDatasetInvalid, Message: "repository is not registered in this service"}}
	}
	if c.store == nil {
		return []DatasetIssue{{Repository: repositoryID, Code: kernel.ErrCapabilityUnsatisfied, Message: "service has no snapshot store"}}
	}
	repo, ok := c.store.Get(repositoryID)
	if !ok {
		return []DatasetIssue{{
			Repository: repositoryID,
			Code:       kernel.ErrUsageInvalid,
			Message:    "repository " + string(repositoryID) + " is not attached",
		}}
	}
	if repo.Archived() {
		return []DatasetIssue{{Repository: repositoryID, Code: kernel.ErrRepositoryArchived, Message: "repository is archived"}}
	}
	if !repo.HasCommit(commit) {
		return []DatasetIssue{{
			Repository: repositoryID,
			Code:       kernel.ErrVersionUnresolved,
			Message:    "commit " + string(commit) + " does not exist in " + string(repositoryID),
		}}
	}
	return nil
}

func datasetCheck(setID string, issues []DatasetIssue) DatasetCheck {
	outcome := "PASSED"
	if len(issues) > 0 {
		outcome = "FAILED"
	}
	return DatasetCheck{SetID: setID, Outcome: outcome, Issues: issues}
}

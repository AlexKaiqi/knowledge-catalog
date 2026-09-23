package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"kc/kernel"
)

// ResolvedKnowledgeSet is one command pin: the published file list at a Dataset
// version (and the {Repository → commit} map that list implies). Taken at
// ResolveKnowledgeSet from frozen source commits, or an overlay for preview.
// Not a registry object. Catalog stops here: no object_id, no event payload.

type ResolvedKnowledgeSet struct {
	SetID    string `json:"setId"`
	Revision int    `json:"revision"`
	// Ref is the published Dataset version label: "vN" for this revision.
	// latest is Catalog.Set (the current maximum revision). One request
	// resolves this once (V-01); storage stays the integer revision.
	Ref          string                                  `json:"ref,omitempty"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	Items        []DatasetItem                           `json:"items,omitempty"`
	// PinID is the content-address of this pin: workspace id, path layout,
	// {Repository→commit}. Revision is a recipe counter and does
	// not participate. Re-export and pass --pin to replay; replay still
	// evaluates allow per member (docs/COMPOSITION.md).
	PinID string `json:"pinId,omitempty"`
}

type KnowledgeSetIssue struct {
	Repository kernel.RepositoryID `json:"repository"`
	Code       kernel.ErrorCode    `json:"code"`
	Message    string              `json:"message"`
}

type KnowledgeSetCheck struct {
	SetID   string              `json:"setId"`
	Outcome string              `json:"outcome"`
	Issues  []KnowledgeSetIssue `json:"issues"`
}

// HashResolved is the content-address of everything that determines what a
// consumer would read at this pin. Revision is excluded: two recipe edits
// that leave membership, layout and commits unchanged are the same pin.
func HashResolved(setID string, sources []KnowledgeSetSource, repos map[kernel.RepositoryID]kernel.CommitID) string {
	keys := make([]string, 0, len(repos))
	for k := range repos {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	byRepo := map[kernel.RepositoryID][]KnowledgeSetSource{}
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
// are legal KnowledgeSetSource.Path values but they are not the same pin.
func mountHashToken(path *string) string {
	if path == nil {
		return "-"
	}
	return normalizeMountPath(*path)
}

func (c *Catalog) ResolveKnowledgeSet(setID string) (ResolvedKnowledgeSet, error) {
	return c.ResolveKnowledgeSetOverlay(setID, nil)
}

func (c *Catalog) ResolveKnowledgeSetOverlay(setID string, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedKnowledgeSet, error) {
	def, err := c.Set(setID)
	if err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	return c.ResolveDefinitionOverlay(def, overlay)
}

func (c *Catalog) ResolveDefinition(def KnowledgeSet) (ResolvedKnowledgeSet, error) {
	return c.ResolveDefinitionOverlay(def, nil)
}

func (c *Catalog) ResolveDefinitionOverlay(def KnowledgeSet, overlay map[kernel.RepositoryID]kernel.CommitID) (ResolvedKnowledgeSet, error) {
	if def.Retired {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "workspace %s is retired", def.SetID)
	}
	if len(def.Sources) == 0 {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "a workspace must contain at least one repository")
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	if err := validateSourceCoordinates(def.Sources); err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		if _, resolved := repositories[src.Repository]; resolved {
			continue
		}
		if err := c.requireRepository(src.Repository); err != nil {
			return ResolvedKnowledgeSet{}, err
		}
		if _, err := c.store.Require(src.Repository, kernel.ErrKnowledgeSetInvalid); err != nil {
			return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "workspace recipe names unknown repository %s", src.Repository)
		}
		repo, _ := c.store.Get(src.Repository)
		if repo.Archived() {
			return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", src.Repository)
		}
		var commit kernel.CommitID
		if overlayCommit, hit := overlay[src.Repository]; hit {
			if !repo.HasCommit(overlayCommit) {
				return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", overlayCommit, src.Repository)
			}
			commit = overlayCommit
		} else if src.Commit != "" {
			if !repo.HasCommit(src.Commit) {
				return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", src.Commit, src.Repository)
			}
			commit = src.Commit
		} else {
			resolved, ok := repo.GetRef(src.Selector)
			if !ok {
				return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s has no ref %s", src.Repository, src.Selector)
			}
			if src.BaseRev != "" {
				want := kernel.CommitID(src.BaseRev)
				if !repo.HasCommit(want) {
					return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrVersionUnresolved, "baseRev %s does not exist in %s", src.BaseRev, src.Repository)
				}
				if resolved != want {
					return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrNonFastForward,
						"repository %s selector %s is at %s, recipe baseRev is %s", src.Repository, src.Selector, resolved, src.BaseRev)
				}
			}
			commit = resolved
		}
		repositories[src.Repository] = commit
	}
	for repositoryID := range overlay {
		if _, ok := repositories[repositoryID]; !ok {
			return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s is not in the workspace", repositoryID)
		}
	}
	items, err := datasetItemsFromSources(def.Sources, repositories)
	if err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	return ResolvedKnowledgeSet{
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

func (c *Catalog) CheckResolved(resolved ResolvedKnowledgeSet) KnowledgeSetCheck {
	issues := []KnowledgeSetIssue{}
	for repositoryID, commit := range resolved.Repositories {
		issues = append(issues, c.checkResolvedRepository(repositoryID, commit)...)
	}
	return knowledgeSetCheck(resolved.SetID, issues)
}

// CheckResolvedRepository validates the one member a path-routed operation
// will actually read. The caller must first validate the pin's Workspace
// identity, membership and PinID against the effective definition. This keeps
// one VFS file read proportional to its owning Repository instead of probing
// every unrelated member in the Workspace.
func (c *Catalog) CheckResolvedRepository(resolved ResolvedKnowledgeSet, repositoryID kernel.RepositoryID) KnowledgeSetCheck {
	commit, ok := resolved.Repositories[repositoryID]
	if !ok {
		return knowledgeSetCheck(resolved.SetID, []KnowledgeSetIssue{{
			Repository: repositoryID,
			Code:       kernel.ErrKnowledgeSetInvalid,
			Message:    "resolved pin has no commit for repository " + string(repositoryID),
		}})
	}
	return knowledgeSetCheck(resolved.SetID, c.checkResolvedRepository(repositoryID, commit))
}

func (c *Catalog) checkResolvedRepository(repositoryID kernel.RepositoryID, commit kernel.CommitID) []KnowledgeSetIssue {
	if !c.HasRepository(repositoryID) {
		return []KnowledgeSetIssue{{Repository: repositoryID, Code: kernel.ErrKnowledgeSetInvalid, Message: "repository is not registered in this catalog"}}
	}
	if c.store == nil {
		return []KnowledgeSetIssue{{Repository: repositoryID, Code: kernel.ErrCapabilityUnsatisfied, Message: "catalog has no snapshot store"}}
	}
	repo, ok := c.store.Get(repositoryID)
	if !ok {
		return []KnowledgeSetIssue{{
			Repository: repositoryID,
			Code:       kernel.ErrUsageInvalid,
			Message:    "repository " + string(repositoryID) + " is not attached",
		}}
	}
	if repo.Archived() {
		return []KnowledgeSetIssue{{Repository: repositoryID, Code: kernel.ErrRepositoryArchived, Message: "repository is archived"}}
	}
	if !repo.HasCommit(commit) {
		return []KnowledgeSetIssue{{
			Repository: repositoryID,
			Code:       kernel.ErrVersionUnresolved,
			Message:    "commit " + string(commit) + " does not exist in " + string(repositoryID),
		}}
	}
	return nil
}

func knowledgeSetCheck(setID string, issues []KnowledgeSetIssue) KnowledgeSetCheck {
	outcome := "PASSED"
	if len(issues) > 0 {
		outcome = "FAILED"
	}
	return KnowledgeSetCheck{SetID: setID, Outcome: outcome, Issues: issues}
}

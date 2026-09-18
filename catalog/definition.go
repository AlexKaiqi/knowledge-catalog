package catalog

import (
	"slices"
	"strings"

	"kc/kernel"
)

// KnowledgeSet is a published Dataset: named file pointers frozen at define
// time. Selectors are resolved to commits on publish; later branch movement
// does not change this version. Republish (new revision) writes the next list.

// KnowledgeSetSource is a Mount when Path is set: repository + selector + where that
// repository's tree lands in a composed workspace tree, plus which subtree of
// it (SubPath). Path nil means this source only feeds federated knowledge
// reads (reader.Open / AccessSpec) and never participates in path-based
// composition or write-back routing — the pre-Loom use of this struct.
// Path "" (a non-nil pointer to the empty string) is the root mount: the
// fallback for files that match no other mount. See docs/COMPOSITION.md.
// One repository may have several Path entries only when they share one
// selector/baseRev and project disjoint SubPaths; the resolved pin still has
// one commit coordinate for that repository.
type KnowledgeSetSource struct {
	Repository kernel.RepositoryID `json:"repository"`
	Selector   string              `json:"selector"`
	Path       *string             `json:"path,omitempty"`
	SubPath    string              `json:"subPath,omitempty"`
	// Commit is the source snapshot frozen at publish. Empty only on
	// unpublished overlay/temporary recipes, which still follow Selector.
	Commit kernel.CommitID `json:"commit,omitempty"`
	// BaseRev is recipe-layer CAS at publish: DefineKnowledgeSet fails
	// NON_FAST_FORWARD if the selector's tip is not this commit. After
	// publish, Resolve uses Commit and ignores later branch movement.
	BaseRev string `json:"baseRev,omitempty"`
}

// MountPath is the *string helper for a KnowledgeSetSource.Path literal, since Go has
// no address-of-literal syntax: Path: catalog.MountPath("refs/semantic").
func MountPath(path string) *string { return &path }

type KnowledgeSet struct {
	SetID    string               `json:"setId"`
	Revision int                  `json:"revision"`
	Sources  []KnowledgeSetSource `json:"sources"`
	Items    []DatasetItem        `json:"items,omitempty"`
	Retired  bool                 `json:"retired,omitempty"`
}

func (c *Catalog) DefineKnowledgeSet(setID string, revision int, sources []KnowledgeSetSource) (KnowledgeSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureWritable(); err != nil {
		return KnowledgeSet{}, err
	}
	if existing, ok := c.datasets[setID]; ok && existing.Retired {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "workspace %s is retired", setID)
	}
	seen := map[kernel.RepositoryID]struct{}{}
	for _, src := range sources {
		if _, ok := seen[src.Repository]; ok {
			continue
		}
		seen[src.Repository] = struct{}{}
		if _, ok := c.repositories[string(src.Repository)]; !ok {
			return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s is not registered in this catalog", src.Repository)
		}
	}
	if err := validateMountPaths(sources); err != nil {
		return KnowledgeSet{}, err
	}
	if err := validateSourceCoordinates(sources); err != nil {
		return KnowledgeSet{}, err
	}
	frozen, err := c.freezeSources(sources)
	if err != nil {
		return KnowledgeSet{}, err
	}
	items, err := datasetItemsFromSources(frozen, nil)
	if err != nil {
		return KnowledgeSet{}, err
	}
	def := KnowledgeSet{SetID: setID, Revision: revision, Sources: frozen, Items: items}
	next := c.dumpState()
	next.KnowledgeSets = slices.DeleteFunc(next.KnowledgeSets, func(existing KnowledgeSet) bool { return existing.SetID == setID })
	next.KnowledgeSets = append(next.KnowledgeSets, cloneKnowledgeSet(def))
	if err := c.persist(next, "dataset-define "+setID); err != nil {
		return KnowledgeSet{}, err
	}
	return cloneKnowledgeSet(def), nil
}

// validateSourceCoordinates lets one repository project several disjoint
// subtrees into different Workspace paths without pretending the same
// repository can be pinned at two commits. Repeated entries are mount-only,
// share selector/baseRev, and may not expose overlapping repository paths.
func validateSourceCoordinates(sources []KnowledgeSetSource) error {
	byRepo := map[kernel.RepositoryID][]KnowledgeSetSource{}
	for _, src := range sources {
		for _, prior := range byRepo[src.Repository] {
			if src.Path == nil || prior.Path == nil {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"repository %s appears twice without explicit mount paths", src.Repository)
			}
			if src.Selector != prior.Selector || src.BaseRev != prior.BaseRev || src.Commit != prior.Commit {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"repository %s has multiple mount paths but different selector/baseRev/commit coordinates", src.Repository)
			}
			a, b := normalizeMemberSubPath(src.SubPath), normalizeMemberSubPath(prior.SubPath)
			if a == b || a == "" || b == "" || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"repository %s mount subPaths %s and %s overlap", src.Repository, memberPathLabel(a), memberPathLabel(b))
			}
		}
		byRepo[src.Repository] = append(byRepo[src.Repository], src)
	}
	return nil
}

func normalizeMemberSubPath(value string) string {
	return normalizeMountPath(value)
}

func memberPathLabel(value string) string {
	if value == "" {
		return "<root>"
	}
	return value
}

func (c *Catalog) Set(setID string) (KnowledgeSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	def, ok := c.datasets[setID]
	if !ok {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s is not defined in this catalog", setID)
	}
	return cloneKnowledgeSet(def), nil
}

func cloneKnowledgeSet(def KnowledgeSet) KnowledgeSet {
	def.Sources = slices.Clone(def.Sources)
	def.Items = slices.Clone(def.Items)
	for i := range def.Sources {
		if def.Sources[i].Path != nil {
			value := *def.Sources[i].Path
			def.Sources[i].Path = &value
		}
	}
	return def
}

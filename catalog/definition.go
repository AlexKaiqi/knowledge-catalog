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
	// File marks a per-file entry (U10): File is the repository-relative
	// source file path and Target is the delivered path in the composed
	// tree. A file entry never declares a mount Path, and every entry of
	// one repository shares the same frozen commit (docs/reviewed/dataset.md,
	// "同一来源的多个片段继续使用一致的固定版本").
	File   string `json:"file,omitempty"`
	Target string `json:"target,omitempty"`
}

// IsFileEntry reports whether this source selects one file for delivery
// instead of a mount subtree. Mount entries carry Path; file entries carry
// File+Target; the two shapes never mix in one entry.
func (s KnowledgeSetSource) IsFileEntry() bool { return s.File != "" }

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
	def, err := c.PrepareKnowledgeSet(setID, revision, sources)
	if err != nil {
		return KnowledgeSet{}, err
	}
	return c.PublishKnowledgeSet(def)
}

// PrepareKnowledgeSet freezes a candidate without changing the serving version.
// Upper layers may prepare derived capabilities before PublishKnowledgeSet.
func (c *Catalog) PrepareKnowledgeSet(setID string, revision int, sources []KnowledgeSetSource) (KnowledgeSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if err := c.ensureWritable(); err != nil {
		return KnowledgeSet{}, err
	}
	if err := c.checkNextRevision(setID, revision); err != nil {
		return KnowledgeSet{}, err
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
	return def, nil
}

func (c *Catalog) checkNextRevision(setID string, revision int) error {
	if strings.TrimSpace(setID) == "" || revision <= 0 {
		return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset identity and a positive revision are required")
	}
	if existing, ok := c.datasets[setID]; ok {
		if existing.Retired {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s is retired", setID)
		}
		if revision <= existing.Revision {
			return kernel.Fail(kernel.ErrNonFastForward, "dataset %s revision must advance beyond v%d", setID, existing.Revision)
		}
	}
	return nil
}

// PublishKnowledgeSet atomically retains an immutable release and advances
// latest. No knowledge/index dependency belongs in this file-level boundary.
func (c *Catalog) PublishKnowledgeSet(def KnowledgeSet) (KnowledgeSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureWritable(); err != nil {
		return KnowledgeSet{}, err
	}
	if err := c.checkNextRevision(def.SetID, def.Revision); err != nil {
		return KnowledgeSet{}, err
	}
	if len(def.Sources) == 0 {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset requires sources")
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return KnowledgeSet{}, err
	}
	if err := validateSourceCoordinates(def.Sources); err != nil {
		return KnowledgeSet{}, err
	}
	for _, src := range def.Sources {
		if _, ok := c.repositories[string(src.Repository)]; !ok || src.Commit == "" {
			return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset publication requires registered, frozen sources")
		}
	}
	frozen, err := c.freezeSources(def.Sources)
	if err != nil {
		return KnowledgeSet{}, err
	}
	items, err := datasetItemsFromSources(frozen, nil)
	if err != nil {
		return KnowledgeSet{}, err
	}
	def = KnowledgeSet{SetID: def.SetID, Revision: def.Revision, Sources: frozen, Items: items}
	next := c.dumpState()
	next.KnowledgeSets = slices.DeleteFunc(next.KnowledgeSets, func(existing KnowledgeSet) bool { return existing.SetID == def.SetID })
	next.KnowledgeSets = append(next.KnowledgeSets, cloneKnowledgeSet(def))
	next.DatasetVersions = append(next.DatasetVersions, cloneKnowledgeSet(def))
	if err := c.persist(next, "dataset-define "+def.SetID); err != nil {
		return KnowledgeSet{}, err
	}
	return cloneKnowledgeSet(def), nil
}

// DatasetVersion resolves a published version while honoring current retirement.
// Repository registration and current authorization are checked at consumption.
func (c *Catalog) DatasetVersion(setID string, revision int) (KnowledgeSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	current, ok := c.datasets[setID]
	if !ok || current.Retired {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s is absent or retired", setID)
	}
	def, ok := c.versions[setID][revision]
	if !ok {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrVersionUnresolved, "dataset %s v%d was not published", setID, revision)
	}
	return cloneKnowledgeSet(def), nil
}

// validateSourceCoordinates lets one repository project several disjoint
// subtrees into different Workspace paths without pretending the same
// repository can be pinned at two commits. Repeated entries — mounts and
// per-file entries alike — share selector/baseRev/commit, and mount subPaths
// may not expose overlapping repository paths.
func validateSourceCoordinates(sources []KnowledgeSetSource) error {
	byRepo := map[kernel.RepositoryID][]KnowledgeSetSource{}
	for _, src := range sources {
		for _, prior := range byRepo[src.Repository] {
			if src.Selector != prior.Selector || src.BaseRev != prior.BaseRev || src.Commit != prior.Commit {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"repository %s has multiple entries but different selector/baseRev/commit coordinates", src.Repository)
			}
			if src.IsFileEntry() || prior.IsFileEntry() {
				// A per-file entry may name a path inside a mounted subtree or
				// alongside one; only whole mount subPaths must stay disjoint.
				continue
			}
			if src.Path == nil || prior.Path == nil {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"repository %s appears twice without explicit mount paths", src.Repository)
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

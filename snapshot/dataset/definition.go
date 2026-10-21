package dataset

import (
	"maps"
	"slices"
	"strings"

	"kc/kernel"
)

// Dataset is a published Dataset: named file pointers frozen at define
// time. Selectors are resolved to commits on publish; later branch movement
// does not change this version. Republish (new revision) writes the next list.

// DatasetSource is a Mount when Path is set: repository + selector + where that
// repository's tree lands in a composed workspace tree, plus which subtree of
// it (SubPath). Path nil means this source has no explicit directory mount. External
// consumers may use its fixed version coordinate, but the file module never
// interprets its body or derives object identity from its path.
// Path "" (a non-nil pointer to the empty string) is the root mount: the
// fallback for files that match no other mount. See doc/design/dataset.md.
// One repository may have several Path entries only when they share one
// selector/baseRev and project disjoint SubPaths; the resolved pin still has
// one commit coordinate for that repository.
type DatasetSource struct {
	Repository kernel.RepositoryID `json:"repository"`
	Selector   string              `json:"selector"`
	Path       *string             `json:"path,omitempty"`
	SubPath    string              `json:"subPath,omitempty"`
	// Commit is the source snapshot frozen at publish. Empty only on
	// unpublished overlay/temporary recipes, which still follow Selector.
	Commit kernel.CommitID `json:"commit,omitempty"`
	// BaseRev is recipe-layer CAS at publish: DefineDataset fails
	// NON_FAST_FORWARD if the selector's tip is not this commit. After
	// publish, Resolve uses Commit and ignores later branch movement.
	BaseRev string `json:"baseRev,omitempty"`
	// File marks a per-file entry (U10): File is the repository-relative
	// source file path and Target is the delivered path in the composed
	// tree. A file entry never declares a mount Path, and every entry of
	// one repository shares the same frozen commit (doc/design/dataset.md,
	// "同一来源的多个片段继续使用一致的固定版本").
	File   string `json:"file,omitempty"`
	Target string `json:"target,omitempty"`
}

// IsFileEntry reports whether this source selects one file for delivery
// instead of a mount subtree. Mount entries carry Path; file entries carry
// File+Target; the two shapes never mix in one entry.
func (s DatasetSource) IsFileEntry() bool { return s.File != "" }

// MountPath is the *string helper for a DatasetSource.Path literal, since Go has
// no address-of-literal syntax: Path: dataset.MountPath("refs/semantic").
func MountPath(path string) *string { return &path }

type Dataset struct {
	SetID    string          `json:"setId"`
	Revision int             `json:"revision"`
	Sources  []DatasetSource `json:"sources"`
	Items    []DatasetItem   `json:"items,omitempty"`
	Retired  bool            `json:"retired,omitempty"`
	// Metadata belongs to this published revision. Keys and values are opaque
	// strings; neither metadata nor its absence changes the delivered file scope.
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	// Dependencies are the Repository versions frozen with this revision only
	// to interpret references (Schema, ResourceDescriptor) held outside the
	// selected sources. The file layer stores them as opaque coordinates: they
	// are not sources, deliver no files and grant no read. Following does not
	// watch them; the next revision freezes them again (doc/design/dataset.md).
	Dependencies map[kernel.RepositoryID]kernel.CommitID `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

func (c *service) DefineDataset(setID string, revision int, sources []DatasetSource) (Dataset, error) {
	def, err := c.PrepareDataset(setID, revision, sources)
	if err != nil {
		return Dataset{}, err
	}
	return c.PublishDataset(def)
}

// PrepareDataset freezes a candidate without changing the serving version.
// Upper layers may prepare derived capabilities before PublishDataset.
// It reads one accepted version and takes no lock, so freezing sources never
// delays readers or other writers.
func (c *service) PrepareDataset(setID string, revision int, sources []DatasetSource) (Dataset, error) {
	if err := c.ensureWritable(); err != nil {
		return Dataset{}, err
	}
	accepted := c.accepted()
	if err := accepted.checkNextRevision(setID, revision); err != nil {
		return Dataset{}, err
	}
	for _, src := range sources {
		if _, ok := accepted.repositories[string(src.Repository)]; !ok {
			return Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not registered in this service", src.Repository)
		}
	}
	if err := validateMountPaths(sources); err != nil {
		return Dataset{}, err
	}
	if err := validateSourceCoordinates(sources); err != nil {
		return Dataset{}, err
	}
	frozen, err := c.freezeSources(sources)
	if err != nil {
		return Dataset{}, err
	}
	items, err := datasetItemsFromSources(frozen, nil)
	if err != nil {
		return Dataset{}, err
	}
	if err := c.validatePublicationFiles(items); err != nil {
		return Dataset{}, err
	}
	def := Dataset{SetID: setID, Revision: revision, Sources: frozen, Items: items}
	return def, nil
}

func (s *acceptedState) checkNextRevision(setID string, revision int) error {
	if strings.TrimSpace(setID) == "" || revision <= 0 {
		return kernel.Fail(kernel.ErrDatasetInvalid, "dataset identity and a positive revision are required")
	}
	if existing, ok := s.datasets[setID]; ok {
		if existing.Retired {
			return kernel.Fail(kernel.ErrDatasetInvalid, "dataset %s is retired", setID)
		}
		if revision <= existing.Revision {
			return kernel.Fail(kernel.ErrNonFastForward, "dataset %s revision must advance beyond v%d", setID, existing.Revision)
		}
	}
	return nil
}

// PublishDataset atomically retains an immutable release and advances
// latest. No object/index dependency belongs in this file-level boundary.
// Source freezing and file validation read immutable commits outside any lock;
// only the in-memory recheck and the registry commit are serialized, and
// readers keep the previous version until the commit is accepted.
func (c *service) PublishDataset(def Dataset) (Dataset, error) {
	if err := c.ensureWritable(); err != nil {
		return Dataset{}, err
	}
	if err := c.checkPublication(c.accepted(), def); err != nil {
		return Dataset{}, err
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return Dataset{}, err
	}
	if err := validateSourceCoordinates(def.Sources); err != nil {
		return Dataset{}, err
	}
	frozen, err := c.freezeSources(def.Sources)
	if err != nil {
		return Dataset{}, err
	}
	items, err := datasetItemsFromSources(frozen, nil)
	if err != nil {
		return Dataset{}, err
	}
	if err := c.validatePublicationFiles(items); err != nil {
		return Dataset{}, err
	}
	if err := c.validateDependencies(frozen, def.Dependencies); err != nil {
		return Dataset{}, err
	}
	def = Dataset{SetID: def.SetID, Revision: def.Revision, Sources: frozen, Items: items, Metadata: maps.Clone(def.Metadata), Dependencies: maps.Clone(def.Dependencies)}
	err = c.commitDataset(def.SetID, "dataset-define "+def.SetID, func(s *acceptedState) (State, error) {
		// The accepted version may have advanced since validation.
		if err := c.checkPublication(s, def); err != nil {
			return State{}, err
		}
		next := s.dump(c.registry.AuthorityID())
		next.Datasets = slices.DeleteFunc(next.Datasets, func(existing Dataset) bool { return existing.SetID == def.SetID })
		next.Datasets = append(next.Datasets, cloneDataset(def))
		next.DatasetVersions = append(next.DatasetVersions, cloneDataset(def))
		return next, nil
	})
	if err != nil {
		return Dataset{}, err
	}
	return cloneDataset(def), nil
}

// checkPublication is the registry-state half of publication validity: the
// revision advances and every source is registered and frozen.
func (c *service) checkPublication(s *acceptedState, def Dataset) error {
	if err := s.checkNextRevision(def.SetID, def.Revision); err != nil {
		return err
	}
	if len(def.Sources) == 0 {
		return kernel.Fail(kernel.ErrDatasetInvalid, "dataset requires sources")
	}
	for _, src := range def.Sources {
		if _, ok := s.repositories[string(src.Repository)]; !ok || src.Commit == "" {
			return kernel.Fail(kernel.ErrDatasetInvalid, "dataset publication requires registered, frozen sources")
		}
	}
	return nil
}

// DatasetVersion resolves a published version while honoring current retirement.
// Repository registration and current authorization are checked at consumption.
func (c *service) DatasetVersion(setID string, revision int) (Dataset, error) {
	accepted := c.accepted()
	current, ok := accepted.datasets[setID]
	if !ok || current.Retired {
		return Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "dataset %s is absent or retired", setID)
	}
	def, ok := accepted.versions[setID][revision]
	if !ok {
		return Dataset{}, kernel.Fail(kernel.ErrVersionUnresolved, "dataset %s v%d was not published", setID, revision)
	}
	return cloneDataset(def), nil
}

// validateSourceCoordinates lets one repository project several disjoint
// subtrees into different Workspace paths without pretending the same
// repository can be pinned at two commits. Repeated entries — mounts and
// per-file entries alike — share selector/baseRev/commit, and mount subPaths
// may not expose overlapping repository paths.
func validateSourceCoordinates(sources []DatasetSource) error {
	byRepo := map[kernel.RepositoryID][]DatasetSource{}
	for _, src := range sources {
		for _, prior := range byRepo[src.Repository] {
			if src.Selector != prior.Selector || src.BaseRev != prior.BaseRev || src.Commit != prior.Commit {
				return kernel.Fail(kernel.ErrDatasetInvalid,
					"repository %s has multiple entries but different selector/baseRev/commit coordinates", src.Repository)
			}
			if src.IsFileEntry() || prior.IsFileEntry() {
				// A per-file entry may name a path inside a mounted subtree or
				// alongside one; only whole mount subPaths must stay disjoint.
				continue
			}
			if src.Path == nil || prior.Path == nil {
				return kernel.Fail(kernel.ErrDatasetInvalid,
					"repository %s appears twice without explicit mount paths", src.Repository)
			}
			a, b := normalizeMemberSubPath(src.SubPath), normalizeMemberSubPath(prior.SubPath)
			if a == b || a == "" || b == "" || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return kernel.Fail(kernel.ErrDatasetInvalid,
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

func (c *service) Set(setID string) (Dataset, error) {
	def, ok := c.accepted().datasets[setID]
	if !ok {
		return Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "dataset %s is not defined in this service", setID)
	}
	return cloneDataset(def), nil
}

func cloneDataset(def Dataset) Dataset {
	def.Metadata = maps.Clone(def.Metadata)
	def.Dependencies = maps.Clone(def.Dependencies)
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

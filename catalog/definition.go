package catalog

import "kc/kernel"

// WorkspaceDefinition is the consumer recipe: which repositories to join, via selectors
// (usually a published branch). Changing it changes the next ResolveWorkspace.
// Publishers move those branches; consumers do not pin a second serving pointer.

// WorkspaceSource is a Mount when Path is set: repository + selector + where that
// repository's tree lands in a composed workspace tree, plus which subtree of
// it (SubPath). Path nil means this source only feeds federated knowledge
// reads (reader.Open / IndexPlan) and never participates in path-based
// composition or write-back routing — the pre-Loom use of this struct.
// Path "" (a non-nil pointer to the empty string) is the root mount: the
// fallback for files that match no other mount. See docs/COMPOSITION.md.
type WorkspaceSource struct {
	Repository kernel.RepositoryID `json:"repository"`
	Selector   string              `json:"selector"`
	Path       *string             `json:"path,omitempty"`
	SubPath    string              `json:"subPath,omitempty"`
	// BaseRev is recipe-layer CAS (docs/COMPOSITION.md, Android repo base-rev):
	// ResolveWorkspace fails NON_FAST_FORWARD if the selector's tip is no
	// longer this commit. Empty means follow the selector live.
	BaseRev string `json:"baseRev,omitempty"`
}

// MountPath is the *string helper for a WorkspaceSource.Path literal, since Go has
// no address-of-literal syntax: Path: catalog.MountPath("refs/semantic").
func MountPath(path string) *string { return &path }

type WorkspaceDefinition struct {
	WorkspaceID string            `json:"workspaceId"`
	Revision    int               `json:"revision"`
	Sources     []WorkspaceSource `json:"sources"`
	Retired     bool              `json:"retired,omitempty"`
}

func (c *Catalog) DefineWorkspace(workspaceID string, revision int, sources []WorkspaceSource) (WorkspaceDefinition, error) {
	if err := c.ensureWritable(); err != nil {
		return WorkspaceDefinition{}, err
	}
	if existing, ok := c.workspaces[workspaceID]; ok && existing.Retired {
		return WorkspaceDefinition{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is retired", workspaceID)
	}
	seen := map[kernel.RepositoryID]struct{}{}
	for _, src := range sources {
		if _, dup := seen[src.Repository]; dup {
			return WorkspaceDefinition{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s appears twice", src.Repository)
		}
		seen[src.Repository] = struct{}{}
		if err := c.requireRepository(src.Repository); err != nil {
			return WorkspaceDefinition{}, err
		}
	}
	if err := validateMountPaths(sources); err != nil {
		return WorkspaceDefinition{}, err
	}
	def := WorkspaceDefinition{WorkspaceID: workspaceID, Revision: revision, Sources: sources}
	c.workspaces[workspaceID] = def
	if err := c.persist("define-workspace " + workspaceID); err != nil {
		return WorkspaceDefinition{}, err
	}
	return def, nil
}

func (c *Catalog) Workspace(workspaceID string) (WorkspaceDefinition, error) {
	def, ok := c.workspaces[workspaceID]
	if !ok {
		return WorkspaceDefinition{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is not defined in this catalog", workspaceID)
	}
	return def, nil
}

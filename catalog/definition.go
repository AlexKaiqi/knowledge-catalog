package catalog

import (
	"strings"

	"kc/kernel"
)

// WorkspaceDefinition is the consumer recipe: which repositories to join, via selectors
// (usually a published branch). Changing it changes the next ResolveWorkspace.
// Publishers move those branches; consumers do not pin a second serving pointer.

// WorkspaceSource is a Mount when Path is set: repository + selector + where that
// repository's tree lands in a composed workspace tree, plus which subtree of
// it (SubPath). Path nil means this source only feeds federated knowledge
// reads (reader.Open / AccessSpec) and never participates in path-based
// composition or write-back routing — the pre-Loom use of this struct.
// Path "" (a non-nil pointer to the empty string) is the root mount: the
// fallback for files that match no other mount. See docs/COMPOSITION.md.
// One repository may have several Path entries only when they share one
// selector/baseRev and project disjoint SubPaths; the resolved pin still has
// one commit coordinate for that repository.
type WorkspaceSource struct {
	Repository kernel.RepositoryID `json:"repository"`
	Selector   string              `json:"selector"`
	Path       *string             `json:"path,omitempty"`
	SubPath    string              `json:"subPath,omitempty"`
	// BaseRev is recipe-layer CAS (docs/COMPOSITION.md, Android Repo base-rev):
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
		if _, ok := seen[src.Repository]; ok {
			continue
		}
		seen[src.Repository] = struct{}{}
		if err := c.requireRepository(src.Repository); err != nil {
			return WorkspaceDefinition{}, err
		}
	}
	if err := validateMountPaths(sources); err != nil {
		return WorkspaceDefinition{}, err
	}
	if err := validateSourceCoordinates(sources); err != nil {
		return WorkspaceDefinition{}, err
	}
	def := WorkspaceDefinition{WorkspaceID: workspaceID, Revision: revision, Sources: sources}
	c.workspaces[workspaceID] = def
	if err := c.persist("define-workspace " + workspaceID); err != nil {
		return WorkspaceDefinition{}, err
	}
	return def, nil
}

// validateSourceCoordinates lets one repository project several disjoint
// subtrees into different Workspace paths without pretending the same
// repository can be pinned at two commits. Repeated entries are mount-only,
// share selector/baseRev, and may not expose overlapping repository paths.
func validateSourceCoordinates(sources []WorkspaceSource) error {
	byRepo := map[kernel.RepositoryID][]WorkspaceSource{}
	for _, src := range sources {
		for _, prior := range byRepo[src.Repository] {
			if src.Path == nil || prior.Path == nil {
				return kernel.Fail(kernel.ErrWorkspaceInvalid,
					"repository %s appears twice without explicit mount paths", src.Repository)
			}
			if src.Selector != prior.Selector || src.BaseRev != prior.BaseRev {
				return kernel.Fail(kernel.ErrWorkspaceInvalid,
					"repository %s has multiple mount paths but different selector/baseRev coordinates", src.Repository)
			}
			a, b := normalizeMemberSubPath(src.SubPath), normalizeMemberSubPath(prior.SubPath)
			if a == b || a == "" || b == "" || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return kernel.Fail(kernel.ErrWorkspaceInvalid,
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

func (c *Catalog) Workspace(workspaceID string) (WorkspaceDefinition, error) {
	def, ok := c.workspaces[workspaceID]
	if !ok {
		return WorkspaceDefinition{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is not defined in this catalog", workspaceID)
	}
	return def, nil
}

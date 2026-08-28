package catalog

import (
	"strings"

	"kc/kernel"
	snapshotpkg "kc/snapshot"
)

// VirtualFile is one raw byte read across a composed Workspace tree: the
// virtual path the caller asked for, which member/commit it routed to, and
// the bytes there. RouteMount decides ownership; this is that same routing
// applied to a read instead of a write-back plan — the primitive a virtual
// filesystem (no real checkout on disk, docs/COMPOSITION.md's TreeStore)
// needs for a single file.
type VirtualFile struct {
	Path       string              `json:"path"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Content    []byte              `json:"content"`
}

// ReadVirtualFile routes path to its owning mount and reads the raw bytes
// there at this ResolveWorkspace's pin. A member without TreeStore fails with
// CAPABILITY_UNSATISFIED naming it, the same seam-reporting pattern as
// Store.Knowledge.
func (c *Catalog) ReadVirtualFile(workspaceID, path string) (VirtualFile, error) {
	def, err := c.Workspace(workspaceID)
	if err != nil {
		return VirtualFile{}, err
	}
	return c.ReadVirtualFileOf(def, path)
}

func (c *Catalog) ReadVirtualFileOf(def WorkspaceDefinition, path string) (VirtualFile, error) {
	resolved, err := c.ResolveDefinition(def)
	if err != nil {
		return VirtualFile{}, err
	}
	return c.ReadVirtualFileAt(def, resolved, path)
}

// ReadVirtualFileAt reads against a caller-supplied command pin. This is the
// VFS equivalent of reader.Open over a ResolvedWorkspace: repeated remote
// filesystem calls can share one snapshot instead of re-following selectors.
func (c *Catalog) ReadVirtualFileAt(def WorkspaceDefinition, resolved ResolvedWorkspace, path string) (VirtualFile, error) {
	route, err := RouteMount(def, path)
	if err != nil {
		return VirtualFile{}, err
	}
	commit, ok := resolved.Repositories[route.Repository]
	if !ok {
		return VirtualFile{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", route.Repository)
	}
	snapshot, err := c.store.Require(route.Repository, kernel.ErrUsageInvalid)
	if err != nil {
		return VirtualFile{}, err
	}
	raw, ok := snapshotpkg.TreeStoreOf(snapshot)
	if !ok {
		return VirtualFile{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not support raw path reads", route.Repository)
	}
	content, err := raw.ReadFile(route.Path, commit)
	if err != nil {
		return VirtualFile{}, err
	}
	return VirtualFile{Path: path, Repository: route.Repository, Commit: commit, Content: content}, nil
}

// VirtualMount is one declared path boundary in a composed Workspace, paired
// with the commit selected for this command's resolved pin. It is metadata for
// explaining the virtual tree; callers must still enforce repository read
// authorization before exposing it.
type VirtualMount struct {
	Path       string              `json:"path"`
	Repository kernel.RepositoryID `json:"repository"`
	Selector   string              `json:"selector"`
	SubPath    string              `json:"subPath,omitempty"`
	Commit     kernel.CommitID     `json:"commit"`
}

// ListVirtualMountsAt describes every declared mount, including empty mounts
// and members without TreeStore. It is recipe/pin metadata, not a claim that
// a file exists at Path.
func ListVirtualMountsAt(def WorkspaceDefinition, resolved ResolvedWorkspace) ([]VirtualMount, error) {
	if err := requireAllMountsDeclared(def.Sources); err != nil {
		return nil, err
	}
	out := make([]VirtualMount, 0, len(def.Sources))
	for _, src := range rootFirst(def.Sources) {
		commit, ok := resolved.Repositories[src.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", src.Repository)
		}
		out = append(out, VirtualMount{
			Path:       normalizeMountPath(*src.Path),
			Repository: src.Repository,
			Selector:   src.Selector,
			SubPath:    strings.Trim(src.SubPath, "/"),
			Commit:     commit,
		})
	}
	return out, nil
}

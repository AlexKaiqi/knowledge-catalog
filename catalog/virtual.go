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

// VirtualEntry is one path in a composed Workspace's virtual listing.
type VirtualEntry struct {
	Path       string              `json:"path"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
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
// and members without TreeStore. Unlike VirtualEntry this is recipe/pin
// metadata, not a claim that a file exists at Path.
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

// ListVirtualFiles lists every raw path across every mount at this
// ResolveWorkspace's pin, translating each member's repo-internal path back to
// its workspace-relative virtual path (RouteMount run in reverse). A mount
// whose member has no TreeStore is left out of the listing, not errored:
// one member's missing capability must not make the rest of the tree
// unlistable — the same "report honestly, do not fail the whole call"
// decision CheckoutMounts already made for Skipped mounts.
func (c *Catalog) ListVirtualFiles(workspaceID string) ([]VirtualEntry, error) {
	def, err := c.Workspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return c.ListVirtualFilesOf(def)
}

func (c *Catalog) ListVirtualFilesOf(def WorkspaceDefinition) ([]VirtualEntry, error) {
	if err := requireAllMountsDeclared(def.Sources); err != nil {
		return nil, err
	}
	resolved, err := c.ResolveDefinition(def)
	if err != nil {
		return nil, err
	}
	return c.ListVirtualFilesAt(def, resolved)
}

// ListVirtualFilesAt is ListVirtualFilesOf fixed to a supplied command pin.
func (c *Catalog) ListVirtualFilesAt(def WorkspaceDefinition, resolved ResolvedWorkspace) ([]VirtualEntry, error) {
	var out []VirtualEntry
	listed := map[kernel.RepositoryID][]string{}
	for _, src := range def.Sources {
		commit, ok := resolved.Repositories[src.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "resolved pin has no commit for repository %s", src.Repository)
		}
		snapshot, err := c.store.Require(src.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return nil, err
		}
		raw, ok := snapshotpkg.TreeStoreOf(snapshot)
		if !ok {
			continue
		}
		files, ok := listed[src.Repository]
		if !ok {
			files, err = raw.ListFiles(commit)
			if err != nil {
				return nil, err
			}
			listed[src.Repository] = files
		}
		for _, f := range files {
			vpath, ok := virtualPathFor(src, f)
			if !ok {
				continue
			}
			out = append(out, VirtualEntry{Path: vpath, Repository: src.Repository, Commit: commit})
		}
	}
	return out, nil
}

// virtualPathFor is RouteMount's inverse for one member path: strip SubPath
// (a file outside the mounted subtree does not surface at all — ok is false),
// then prepend the mount's Path. Mirrors RouteMount's joinSubPath exactly, in
// reverse.
func virtualPathFor(src WorkspaceSource, repoPath string) (string, bool) {
	sub := strings.Trim(src.SubPath, "/")
	rel := repoPath
	if sub != "" {
		switch {
		case rel == sub:
			rel = ""
		case strings.HasPrefix(rel, sub+"/"):
			rel = strings.TrimPrefix(rel, sub+"/")
		default:
			return "", false
		}
	}
	norm := normalizeMountPath(*src.Path)
	switch {
	case norm == "":
		return rel, true
	case rel == "":
		return norm, true
	default:
		return norm + "/" + rel, true
	}
}

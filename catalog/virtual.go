package catalog

import (
	"path"
	"sort"
	"strings"

	"kc/kernel"
	snapshotpkg "kc/snapshot"
)

// VirtualFile is one raw byte read across a composed Workspace tree: the
// virtual path the caller asked for, which member/commit it routed to, and
// the bytes there. RouteMount decides ownership; this is that same routing
// applied to a read instead of a write-back plan — the primitive a virtual
// filesystem (no real checkout on disk, docs/reviewed/dataset.md's TreeStore)
// needs for a single file.
type VirtualFile struct {
	Path       string              `json:"path"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Content    []byte              `json:"content"`
}

// ReadVirtualFile routes path to its owning mount and reads the raw bytes
// there at this ResolveKnowledgeSet's pin. A member without TreeStore fails with
// CAPABILITY_UNSATISFIED naming it, the same seam-reporting pattern as
// Store.Knowledge.
func (c *Catalog) ReadVirtualFile(setID, path string) (VirtualFile, error) {
	def, err := c.Set(setID)
	if err != nil {
		return VirtualFile{}, err
	}
	return c.ReadVirtualFileOf(def, path)
}

func (c *Catalog) ReadVirtualFileOf(def KnowledgeSet, path string) (VirtualFile, error) {
	resolved, err := c.ResolveDefinition(def)
	if err != nil {
		return VirtualFile{}, err
	}
	return c.ReadVirtualFileAt(def, resolved, path)
}

// ReadVirtualFileAt reads against a caller-supplied command pin. This is the
// VFS equivalent of reader.Open over a ResolvedKnowledgeSet: repeated remote
// filesystem calls can share one snapshot instead of re-following selectors.
func (c *Catalog) ReadVirtualFileAt(def KnowledgeSet, resolved ResolvedKnowledgeSet, path string) (VirtualFile, error) {
	route, err := RouteMount(def, path)
	if err != nil {
		return VirtualFile{}, err
	}
	commit, ok := resolved.Repositories[route.Repository]
	if !ok {
		return VirtualFile{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "resolved pin has no commit for repository %s", route.Repository)
	}
	snapshot, err := c.store.Require(route.Repository, kernel.ErrUsageInvalid)
	if err != nil {
		return VirtualFile{}, err
	}
	raw, ok := snapshotpkg.TreeReaderOf(snapshot)
	if !ok {
		return VirtualFile{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not support raw path reads", route.Repository)
	}
	if !DatasetPathAllowed(resolved.Items, route.Repository, route.Path) {
		return VirtualFile{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "path %s is not in dataset %s", path, def.SetID)
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
// and members without TreeStore. Per-file entries are not mounts and never
// appear here; the file gateway composes them through DatasetDeliveredTree.
// It is recipe/pin metadata, not a claim that a file exists at Path.
func ListVirtualMountsAt(def KnowledgeSet, resolved ResolvedKnowledgeSet) ([]VirtualMount, error) {
	mounts := make([]KnowledgeSetSource, 0, len(def.Sources))
	for _, src := range def.Sources {
		if !src.IsFileEntry() {
			mounts = append(mounts, src)
		}
	}
	if err := requireAllMountsDeclared(mounts); err != nil {
		return nil, err
	}
	out := make([]VirtualMount, 0, len(mounts))
	for _, src := range rootFirst(mounts) {
		commit, ok := resolved.Repositories[src.Repository]
		if !ok {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "resolved pin has no commit for repository %s", src.Repository)
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

// DatasetFileItems returns the per-file entries of a published item list,
// ordered by delivered path. Mount (prefix) entries are excluded: they
// deliver whole directories, not named files.
func DatasetFileItems(items []DatasetItem) []DatasetItem {
	out := make([]DatasetItem, 0, len(items))
	for _, item := range items {
		if item.Kind == DatasetItemFile {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}

// DatasetDeliveredTree answers delivered-path questions for the per-file
// entries of one published item list: which file entries land directly in a
// delivered directory, which directories exist only because file entries name
// them, and which entry delivers an exact path. Mount (prefix) entries are
// not modeled here; the file gateway composes them with this tree.
type DatasetDeliveredTree struct {
	files []DatasetItem
	dirs  map[string]struct{}
}

// NewDatasetDeliveredTree indexes the file entries of items by delivered path.
func NewDatasetDeliveredTree(items []DatasetItem) *DatasetDeliveredTree {
	t := &DatasetDeliveredTree{dirs: map[string]struct{}{}}
	for _, item := range items {
		if item.Kind != DatasetItemFile || normalizeMountPath(item.Target) == "" {
			continue
		}
		t.files = append(t.files, item)
		for dir := path.Dir(normalizeMountPath(item.Target)); dir != "" && dir != "."; dir = path.Dir(dir) {
			t.dirs[dir] = struct{}{}
		}
	}
	sort.Slice(t.files, func(i, j int) bool { return t.files[i].Target < t.files[j].Target })
	return t
}

// Find returns the file entry delivered exactly at target (normalized), if any.
func (t *DatasetDeliveredTree) Find(target string) (DatasetItem, bool) {
	target = normalizeMountPath(target)
	if target == "" {
		return DatasetItem{}, false
	}
	for _, item := range t.files {
		if item.Target == target {
			return item, true
		}
	}
	return DatasetItem{}, false
}

// FilesIn returns the file entries delivered directly inside dir (normalized;
// "" is the delivered root), ordered by delivered path.
func (t *DatasetDeliveredTree) FilesIn(dir string) []DatasetItem {
	dir = normalizeMountPath(dir)
	out := make([]DatasetItem, 0)
	for _, item := range t.files {
		parent := path.Dir(item.Target)
		if parent == "." {
			parent = ""
		}
		if parent == dir {
			out = append(out, item)
		}
	}
	return out
}

// DirsIn returns the immediate delivered subdirectories of dir that exist
// because file entries live below them, ordered by name. It never reports a
// directory that only a mount delivers.
func (t *DatasetDeliveredTree) DirsIn(dir string) []string {
	dir = normalizeMountPath(dir)
	seen := map[string]struct{}{}
	for target := range t.dirs {
		rel := ""
		switch {
		case dir == "":
			rel = target
		case strings.HasPrefix(target, dir+"/"):
			rel = strings.TrimPrefix(target, dir+"/")
		default:
			continue
		}
		name := strings.Split(rel, "/")[0]
		seen[name] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

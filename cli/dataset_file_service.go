package cli

import (
	"encoding/json"
	"path"
	"sort"
	"strings"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

type workspaceFileCoordinate struct {
	Catalog string          `json:"catalog,omitempty"`
	Dataset string          `json:"dataset"`
	Pin     json.RawMessage `json:"pin,omitempty"`
	View    string          `json:"view,omitempty"`
}

type workspaceFileMountsRequest struct {
	workspaceFileCoordinate
}

type workspaceFileDirectoryRequest struct {
	workspaceFileCoordinate
	// MountPath+Directory address one mount-relative subtree (kcfs shape).
	// Path addresses the composed delivered tree directly, which is how
	// per-file entries (U10) outside any mount stay reachable. The two
	// addressing modes are mutually exclusive.
	MountPath    string `json:"mountPath"`
	Directory    string `json:"directory,omitempty"`
	Path         string `json:"path,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type workspaceFileReadRequest struct {
	workspaceFileCoordinate
	MountPath string `json:"mountPath"`
	File      string `json:"file"`
	// Path addresses one delivered file in the composed tree (see
	// workspaceFileDirectoryRequest). MountPath+File remain the mount shape.
	Path   string `json:"path,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
}

type workspaceFileMountsResponse struct {
	Pin    catalog.ResolvedKnowledgeSet `json:"pin"`
	Mounts []catalog.VirtualMount       `json:"mounts"`
}

type workspaceFileDirectoryResponse struct {
	Pin          catalog.ResolvedKnowledgeSet `json:"pin"`
	Mount        catalog.VirtualMount         `json:"mount"`
	Entries      []snapshot.DirectoryEntry    `json:"entries"`
	Continuation string                       `json:"continuation,omitempty"`
	Exhausted    bool                         `json:"exhausted"`
}

type workspaceFileReadResponse struct {
	Pin        catalog.ResolvedKnowledgeSet `json:"pin"`
	Mount      catalog.VirtualMount         `json:"mount"`
	File       string                       `json:"file"`
	// Path echoes the delivered path: mount-relative reads report
	// mountPath/file joined; delivered reads echo the requested path.
	Path       string                `json:"path,omitempty"`
	Item       *catalog.DatasetItem  `json:"item,omitempty"`
	Offset     int64                 `json:"offset"`
	TotalBytes int64                 `json:"totalBytes"`
	EOF        bool                  `json:"eof"`
	Content    []byte                `json:"content"`
}

type workspaceFileView struct {
	home     string
	flags    map[string]FlagValue
	opened   *Home
	pin      catalog.ResolvedKnowledgeSet
	mounts   []catalog.VirtualMount
	tree     *catalog.DatasetDeliveredTree
	visible  map[string]bool
	semantic bool
}

// openWorkspaceFileView borrows the facade-owned Home. The caller keeps its
// invocation read lock until all file work finishes; a view never closes Home.
func openWorkspaceFileView(opened *Home, principal string, coordinate workspaceFileCoordinate, requirePin bool, observe authorizationObserver) (*workspaceFileView, error) {
	if opened == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Knowledge Set File Gateway requires an opened deployment")
	}
	home := opened.Dir
	if strings.TrimSpace(coordinate.Dataset) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "workspace is required")
	}
	if requirePin && len(coordinate.Pin) == 0 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "a fixed ResolvedKnowledgeSet pin is required")
	}
	flags := compactFlags(map[string]FlagValue{
		"home": home, "as": principal, "catalog": coordinate.Catalog, "dataset": coordinate.Dataset,
	})
	if len(coordinate.Pin) > 0 {
		flags["pin"] = string(coordinate.Pin)
	}
	if err := prepareKnowledgePinContext(flags); err != nil {
		return nil, err
	}
	if coordinate.Catalog == "" && len(opened.File.Catalogs) > 0 {
		flags["_default-catalog"] = opened.File.Catalogs[0].ID
	}
	if err := authorize(home, "dataset.resolve", flags, observe); err != nil {
		return nil, err
	}
	cat, err := pickCatalog(opened, flags)
	if err != nil {
		return nil, err
	}
	definition, err := ensureWorkspace(opened, home, cat, coordinate.Dataset)
	if err != nil {
		return nil, err
	}
	pin, err := resolveOrReplay(opened, home, cat, coordinate.Dataset, flags)
	if err != nil {
		return nil, err
	}
	if len(coordinate.Pin) > 0 {
		definition, err = cat.DatasetVersion(coordinate.Dataset, pin.Revision)
		if err != nil {
			return nil, err
		}
	}
	semantic := coordinate.View == semanticFileViewV1 || coordinate.View == "semantic"
	if coordinate.View != "" && !semantic && coordinate.View != "repository" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "view must be repository or semantic")
	}
	var mounts []catalog.VirtualMount
	if semantic {
		mounts = semanticMounts(definition, pin)
	} else {
		mounts, err = catalog.ListVirtualMountsAt(definition, pin)
		if err != nil {
			return nil, err
		}
	}
	visible := map[string]bool{}
	filtered := mounts[:0]
	for _, mount := range mounts {
		allowed, allowErr := datasetFSMayReadRepository(home, flags, string(mount.Repository), opened)
		if allowErr != nil {
			return nil, allowErr
		}
		if allowed {
			visible[mount.Path] = true
			filtered = append(filtered, mount)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Path < filtered[j].Path })
	view := &workspaceFileView{home: home, flags: flags, opened: opened, pin: pin, mounts: filtered, visible: visible, semantic: semantic}
	if !semantic {
		// Per-file entries are part of the same published file list. They get
		// the identical per-repository authorization check as mounts, so an
		// unauthorized member's files neither read nor shape directories.
		items := pin.Items
		if len(items) == 0 {
			items = definition.Items
		}
		fileItems := make([]catalog.DatasetItem, 0, len(items))
		for _, item := range items {
			if item.Kind != catalog.DatasetItemFile {
				continue
			}
			allowed, allowErr := datasetFSMayReadRepository(home, flags, string(item.Repository), opened)
			if allowErr != nil {
				return nil, allowErr
			}
			if allowed {
				fileItems = append(fileItems, item)
			}
		}
		view.tree = catalog.NewDatasetDeliveredTree(fileItems)
	}
	if semantic {
		// Building is part of the explicit attach/mount operation. Directory and
		// file interactions after readiness only read the immutable cached view.
		for _, mount := range filtered {
			repo, requireErr := opened.Reader.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
			if requireErr != nil {
				return nil, requireErr
			}
			store, storeErr := opened.Store.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
			if storeErr != nil {
				return nil, storeErr
			}
			if _, projectionErr := semanticProjectionFor(store, repo, pin); projectionErr != nil {
				return nil, projectionErr
			}
		}
	}
	return view, nil
}

func (v *workspaceFileView) mount(value string) (catalog.VirtualMount, error) {
	clean := strings.Trim(path.Clean("/"+value), "/")
	for _, mount := range v.mounts {
		if mount.Path == clean {
			return mount, nil
		}
	}
	return catalog.VirtualMount{}, kernel.Fail(kernel.ErrForbidden, "mount %s is not present or not authorized", clean)
}

// mountOwning returns the visible mount whose subtree contains delivered path
// p: the mount at p, the mount p lives below, or the root mount (Path "")
// that acts as the fallback owner. Longest declared path wins; mounts never
// nest each other, so at most the root and one subtree mount can match.
func (v *workspaceFileView) mountOwning(p string) (catalog.VirtualMount, bool) {
	best := catalog.VirtualMount{}
	found := false
	for _, mount := range v.mounts {
		if mount.Path != p && mount.Path != "" && !strings.HasPrefix(p, mount.Path+"/") {
			continue
		}
		if !found || len(mount.Path) > len(best.Path) {
			best, found = mount, true
		}
	}
	return best, found
}

// readMountDirectory pages one mount's repository directory at the pinned
// commit and rejects pages whose generation moved under the fixed pin.
func (v *workspaceFileView) readMountDirectory(mount catalog.VirtualMount, directory string, request workspaceFileDirectoryRequest) (snapshot.DirectoryPage, error) {
	resolved, err := datasetFSRepositoryPath(mount.SubPath, directory)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	store, ok := v.opened.Store.Get(mount.Repository)
	if !ok {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
	}
	reader, ok := snapshot.DirectoryReaderOf(store)
	if !ok {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not support directory paging", mount.Repository)
	}
	page, err := reader.ReadDirectory(snapshot.DirectoryRequest{
		Commit: mount.Commit, Directory: resolved, Limit: request.Limit, Continuation: request.Continuation,
	})
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	if page.Generation != string(mount.Commit) {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "directory page generation moved from fixed pin")
	}
	return page, nil
}

func (v *workspaceFileView) list(request workspaceFileDirectoryRequest) (workspaceFileDirectoryResponse, error) {
	// Delivered addressing is "MountPath empty": the delivered tree is
	// addressed by Path alone and Path "" is the root. Mount-relative
	// addressing always names a mount, so the two never collide.
	if request.MountPath == "" {
		return v.listDelivered(request)
	}
	mount, err := v.mount(request.MountPath)
	if err != nil {
		return workspaceFileDirectoryResponse{}, err
	}
	if v.semantic {
		repo, requireErr := v.opened.Reader.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
		if requireErr != nil {
			return workspaceFileDirectoryResponse{}, requireErr
		}
		store, storeErr := v.opened.Store.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
		if storeErr != nil {
			return workspaceFileDirectoryResponse{}, storeErr
		}
		projection, projectionErr := semanticProjectionFor(store, repo, v.pin)
		if projectionErr != nil {
			return workspaceFileDirectoryResponse{}, projectionErr
		}
		entries, continuation, exhausted, listErr := projection.list(request.Directory, request.Limit, request.Continuation)
		return workspaceFileDirectoryResponse{Pin: v.pin, Mount: mount, Entries: entries, Continuation: continuation, Exhausted: exhausted}, listErr
	}
	page, err := v.readMountDirectory(mount, request.Directory, request)
	if err != nil {
		return workspaceFileDirectoryResponse{}, err
	}
	return workspaceFileDirectoryResponse{Pin: v.pin, Mount: mount, Entries: page.Entries, Continuation: page.Continuation, Exhausted: page.Exhausted}, nil
}

// listDelivered enumerates one delivered directory of the composed tree. A
// directory inside a mount subtree pages that member's source directory and
// merges per-file entries into its first page; a directory that exists only
// through per-file entries, or that parents mount paths, lists those directly
// without paging. A name shared between a per-file entry and delivered source
// content is a conflict, never an order-based override
// (docs/reviewed/dataset.md, "冲突不得按挂载顺序覆盖").
func (v *workspaceFileView) listDelivered(request workspaceFileDirectoryRequest) (workspaceFileDirectoryResponse, error) {
	if v.semantic {
		return workspaceFileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered paths address the repository view; the semantic view has no per-file entries")
	}
	if request.MountPath != "" || request.Directory != "" {
		return workspaceFileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered listing takes path alone; mountPath and directory address one mount")
	}
	dir := catalog.NormalizeMountPath(request.Path)
	owning, owns := v.mountOwning(dir)
	if owns {
		return v.listDeliveredMount(dir, owning, request)
	}
	return v.listDeliveredDerived(dir)
}

func (v *workspaceFileView) listDeliveredMount(dir string, mount catalog.VirtualMount, request workspaceFileDirectoryRequest) (workspaceFileDirectoryResponse, error) {
	relative := ""
	if dir != mount.Path {
		relative = strings.TrimPrefix(dir, mount.Path+"/")
	}
	page, err := v.readMountDirectory(mount, relative, request)
	if err != nil {
		return workspaceFileDirectoryResponse{}, err
	}
	entries := page.Entries
	if request.Continuation == "" {
		claimed := map[string]struct{}{}
		for _, entry := range entries {
			claimed[entry.Name] = struct{}{}
		}
		for _, item := range v.tree.FilesIn(dir) {
			name := path.Base(item.Target)
			if _, clash := claimed[name]; clash {
				return workspaceFileDirectoryResponse{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"delivered path %s has conflicting sources", item.Target)
			}
			claimed[name] = struct{}{}
			entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: "file"})
		}
	}
	return workspaceFileDirectoryResponse{
		Pin: v.pin, Mount: mount, Entries: entries, Continuation: page.Continuation, Exhausted: page.Exhausted,
	}, nil
}

// listDeliveredDerived lists a delivered directory that no mount subtree
// pages: mount paths and per-file entries below it become direct entries.
// Mount carries the delivered coordinate only; no single member owns it.
func (v *workspaceFileView) listDeliveredDerived(dir string) (workspaceFileDirectoryResponse, error) {
	claimed := map[string]string{}
	claim := func(name, kind string) error {
		if prior, clash := claimed[name]; clash && prior != kind {
			return kernel.Fail(kernel.ErrKnowledgeSetInvalid, "delivered path %s/%s has conflicting sources", mountLabelFor(dir), name)
		}
		claimed[name] = kind
		return nil
	}
	below := func(target string) (string, bool) {
		if dir == "" {
			return target, true
		}
		return strings.CutPrefix(target, dir+"/")
	}
	for _, mount := range v.mounts {
		if rel, ok := below(mount.Path); ok && rel != "" {
			if err := claim(strings.Split(rel, "/")[0], "directory"); err != nil {
				return workspaceFileDirectoryResponse{}, err
			}
		}
	}
	for _, item := range v.tree.FilesIn(dir) {
		if err := claim(path.Base(item.Target), "file"); err != nil {
			return workspaceFileDirectoryResponse{}, err
		}
	}
	for _, name := range v.tree.DirsIn(dir) {
		if err := claim(name, "directory"); err != nil {
			return workspaceFileDirectoryResponse{}, err
		}
	}
	entries := make([]snapshot.DirectoryEntry, 0, len(claimed))
	for name, kind := range claimed {
		entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: kind})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return workspaceFileDirectoryResponse{
		Pin: v.pin, Mount: catalog.VirtualMount{Path: dir}, Entries: entries, Exhausted: true,
	}, nil
}

func mountLabelFor(dir string) string {
	if dir == "" {
		return "<root>"
	}
	return dir
}

func (v *workspaceFileView) read(request workspaceFileReadRequest) (workspaceFileReadResponse, error) {
	// Delivered addressing is "MountPath empty" (see list): Path alone names
	// the composed tree, root included for directory reads.
	if request.MountPath == "" {
		return v.readDelivered(request)
	}
	mount, err := v.mount(request.MountPath)
	if err != nil {
		return workspaceFileReadResponse{}, err
	}
	if request.Offset < 0 || request.Length < 0 || request.Length > 4<<20 {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "file range must use a non-negative offset and length no greater than 4 MiB")
	}
	length := request.Length
	if length == 0 {
		length = 512 << 10
	}
	var content []byte
	var readErr error
	if v.semantic {
		var repo knowledge.Repository
		repo, readErr = v.opened.Reader.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
		if readErr == nil {
			var projection *semanticProjection
			store, storeErr := v.opened.Store.Require(mount.Repository, kernel.ErrCapabilityUnsatisfied)
			if storeErr != nil {
				return workspaceFileReadResponse{}, storeErr
			}
			projection, readErr = semanticProjectionFor(store, repo, v.pin)
			if readErr == nil {
				content, readErr = projection.read(request.File)
			}
		}
	} else {
		var repositoryPath string
		repositoryPath, readErr = datasetFSRepositoryPath(mount.SubPath, request.File)
		if readErr == nil {
			store, ok := v.opened.Store.Get(mount.Repository)
			if !ok {
				readErr = kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
			} else if tree, ok := snapshot.TreeReaderOf(store); !ok {
				readErr = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support fixed file reads", mount.Repository)
			} else {
				content, readErr = tree.ReadFile(repositoryPath, mount.Commit)
			}
		}
	}
	result := map[string]any{"mountPath": mount.Path, "file": request.File, "repository": mount.Repository, "commit": mount.Commit}
	readFlags := make(map[string]FlagValue, len(v.flags)+2)
	for name, value := range v.flags {
		readFlags[name] = value
	}
	readFlags["repo"] = string(mount.Repository)
	readFlags["path"] = path.Join(mount.Path, request.File)
	readFlags["_action"] = "file.read"
	if _, _, accessErr := recordKnowledgeAccess(v.home, "file-read", readFlags, result, readErr); accessErr != nil && readErr == nil {
		return workspaceFileReadResponse{}, accessErr
	}
	if readErr != nil {
		return workspaceFileReadResponse{}, readErr
	}
	start := request.Offset
	if start > int64(len(content)) {
		start = int64(len(content))
	}
	end := start + int64(length)
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return workspaceFileReadResponse{
		Pin: v.pin, Mount: mount, File: request.File, Offset: start, TotalBytes: int64(len(content)),
		EOF: end == int64(len(content)), Content: content[start:end],
	}, nil
}

// readDelivered serves one delivered file of the composed tree. A per-file
// entry is probed against its owning mount first: when the member's own
// content also delivers the same delivered path the layout has two sources
// for one file and the read is refused; a missing member file lets the
// per-file entry serve. Paths without a per-file entry route into their
// owning mount like the mount-relative shape does.
func (v *workspaceFileView) readDelivered(request workspaceFileReadRequest) (workspaceFileReadResponse, error) {
	if v.semantic {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered paths address the repository view; the semantic view has no per-file entries")
	}
	if request.MountPath != "" || request.File != "" {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered read takes path alone; mountPath and file address one mount")
	}
	if request.Offset < 0 || request.Length < 0 || request.Length > 4<<20 {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "file range must use a non-negative offset and length no greater than 4 MiB")
	}
	delivered := catalog.NormalizeMountPath(request.Path)
	if delivered == "" {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered file path is required")
	}
	item, isFileEntry := v.tree.Find(delivered)
	if !isFileEntry {
		return v.readDeliveredMount(delivered, request)
	}
	if owning, owns := v.mountOwning(delivered); owns {
		relative := ""
		if delivered != owning.Path {
			relative = strings.TrimPrefix(delivered, owning.Path+"/")
		}
		probePath, pathErr := datasetFSRepositoryPath(owning.SubPath, relative)
		if pathErr == nil {
			// Readability probe only: a readable member file at the same
			// delivered path means conflicting sources. The probe content
			// itself is discarded.
			if _, probeErr := v.readMemberFile(owning.Repository, probePath, owning.Commit); probeErr == nil {
				return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"delivered path %s has conflicting sources", delivered)
			} else if kernel.CodeOf(probeErr) != kernel.ErrKnowledgeRefUnresolved {
				return workspaceFileReadResponse{}, probeErr
			}
		}
	}
	content, readErr := v.readMemberFile(item.Repository, item.File, item.Commit)
	mount := catalog.VirtualMount{
		Path:       deliveredParent(delivered),
		Repository: item.Repository,
		Commit:     item.Commit,
	}
	deliveredItem := item
	if err := v.recordDeliveredRead(delivered, item.Repository, item.Commit, content, readErr); err != nil && readErr == nil {
		return workspaceFileReadResponse{}, err
	}
	if readErr != nil {
		return workspaceFileReadResponse{}, readErr
	}
	start := request.Offset
	if start > int64(len(content)) {
		start = int64(len(content))
	}
	// A missing length means "to the end of the file", same default the
	// mount-relative read applies; Length 0 must not read an empty range.
	length := request.Length
	if length == 0 {
		length = 512 << 10
	}
	end := start + int64(length)
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return workspaceFileReadResponse{
		Pin: v.pin, Mount: mount, File: item.File, Path: delivered, Item: &deliveredItem,
		Offset: start, TotalBytes: int64(len(content)), EOF: end == int64(len(content)),
		Content: content[start:end],
	}, nil
}

// readDeliveredMount serves a delivered path that belongs to a mount subtree
// by mapping it back to the member path, then reusing the mount-relative
// read. Response Path keeps the delivered coordinate the caller asked for.
func (v *workspaceFileView) readDeliveredMount(delivered string, request workspaceFileReadRequest) (workspaceFileReadResponse, error) {
	owning, owns := v.mountOwning(delivered)
	if !owns {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrForbidden, "delivered path %s is not present or not authorized", delivered)
	}
	relative := ""
	if delivered != owning.Path {
		relative = strings.TrimPrefix(delivered, owning.Path+"/")
	}
	mounted := workspaceFileReadRequest{
		workspaceFileCoordinate: request.workspaceFileCoordinate,
		MountPath:               owning.Path,
		File:                    relative,
		Offset:                  request.Offset,
		Length:                  request.Length,
	}
	out, err := v.read(mounted)
	if err != nil {
		return out, err
	}
	out.Path = delivered
	return out, nil
}

func (v *workspaceFileView) readMemberFile(repository kernel.RepositoryID, memberPath string, commit kernel.CommitID) ([]byte, error) {
	store, ok := v.opened.Store.Get(repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", repository)
	}
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support fixed file reads", repository)
	}
	return tree.ReadFile(memberPath, commit)
}

func (v *workspaceFileView) recordDeliveredRead(delivered string, repository kernel.RepositoryID, commit kernel.CommitID, content []byte, readErr error) error {
	result := map[string]any{"path": delivered, "repository": repository, "commit": commit, "bytes": len(content)}
	readFlags := make(map[string]FlagValue, len(v.flags)+2)
	for name, value := range v.flags {
		readFlags[name] = value
	}
	readFlags["path"] = delivered
	readFlags["repo"] = string(repository)
	readFlags["_action"] = "file.read"
	if _, _, err := recordKnowledgeAccess(v.home, "file-read", readFlags, result, readErr); err != nil {
		return err
	}
	return nil
}

func deliveredParent(delivered string) string {
	parent := path.Dir(delivered)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

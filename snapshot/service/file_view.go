package service

import (
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"path"
	"sort"
	"strings"
)

// ViewOptions supplies a pre-authorized immutable file scope. HTTP callers
// cannot supply these options; Resources.OpenFiles reconstructs them from the
// accepted Dataset and current policy. External views may supply byte/paging
// ports without placing their interpretation in the file module.
type ViewOptions struct {
	Pin       dataset.ResolvedKnowledgeSet
	Mounts    []dataset.VirtualMount
	Items     []dataset.DatasetItem
	Sources   Sources
	Delivered bool
	Directory func(dataset.VirtualMount, string, int, string) (snapshot.DirectoryPage, error)
	Read      func(dataset.VirtualMount, string) ([]byte, error)
	Record    func(FileReadEvidence) error
}

type fileView struct {
	pin        dataset.ResolvedKnowledgeSet
	mounts     []dataset.VirtualMount
	tree       dataset.DatasetDeliveredTree
	sources    Sources
	delivered  bool
	directory  func(dataset.VirtualMount, string, int, string) (snapshot.DirectoryPage, error)
	sourceRead func(dataset.VirtualMount, string) ([]byte, error)
	record     func(FileReadEvidence) error
}

func NewFileView(options ViewOptions) FileView {
	return &fileView{pin: options.Pin, mounts: append([]dataset.VirtualMount{}, options.Mounts...), tree: dataset.NewDatasetDeliveredTree(dataset.DatasetFileItems(options.Items)), sources: options.Sources, delivered: options.Delivered, directory: options.Directory, sourceRead: options.Read, record: options.Record}
}
func (v *fileView) FixedPin() dataset.ResolvedKnowledgeSet { return v.pin }
func (v *fileView) VisibleMounts() []dataset.VirtualMount {
	return append([]dataset.VirtualMount{}, v.mounts...)
}
func (v *fileView) readSource(mount dataset.VirtualMount, file string) ([]byte, error) {
	if v.sourceRead != nil {
		return v.sourceRead(mount, file)
	}
	repositoryPath, err := RepositoryPath(mount.SubPath, file)
	if err != nil {
		return nil, err
	}
	return v.readMemberFile(mount.Repository, repositoryPath, mount.Commit)
}

func (v *fileView) mount(value string) (dataset.VirtualMount, error) {
	clean := strings.Trim(path.Clean("/"+value), "/")
	for _, mount := range v.mounts {
		if mount.Path == clean {
			return mount, nil
		}
	}
	return dataset.VirtualMount{}, kernel.Fail(kernel.ErrForbidden, "mount %s is not present or not authorized", clean)
}

// mountOwning returns the visible mount whose subtree contains delivered path
// p: the mount at p, the mount p lives below, or the root mount (Path "")
// that acts as the fallback owner. Longest declared path wins; mounts never
// nest each other, so at most the root and one subtree mount can match.
func (v *fileView) mountOwning(p string) (dataset.VirtualMount, bool) {
	best := dataset.VirtualMount{}
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
func (v *fileView) readMountDirectory(mount dataset.VirtualMount, directory string, request FileDirectoryRequest) (snapshot.DirectoryPage, error) {
	if v.directory != nil {
		return v.directory(mount, directory, request.Limit, request.Continuation)
	}
	resolved, err := RepositoryPath(mount.SubPath, directory)
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	store, ok := v.sources.Get(mount.Repository)
	if !ok {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
	}
	reader, ok := snapshot.DirectoryReaderOf(store)
	if !ok {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support directory paging", mount.Repository)
	}
	page, err := reader.ReadDirectory(snapshot.DirectoryRequest{Commit: mount.Commit, Directory: resolved, Limit: request.Limit, Continuation: request.Continuation})
	if err != nil {
		return snapshot.DirectoryPage{}, err
	}
	if page.Generation != string(mount.Commit) {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "directory page generation moved from fixed pin")
	}
	return page, nil
}

func (v *fileView) List(request FileDirectoryRequest) (FileDirectoryResponse, error) {
	// Delivered addressing is "MountPath empty": the delivered tree is
	// addressed by Path alone and Path "" is the root. Mount-relative
	// addressing always names a mount, so the two never collide.
	if request.MountPath == "" {
		return v.listDelivered(request)
	}
	mount, err := v.mount(request.MountPath)
	if err != nil {
		return FileDirectoryResponse{}, err
	}

	page, err := v.readMountDirectory(mount, request.Directory, request)
	if err != nil {
		return FileDirectoryResponse{}, err
	}
	return FileDirectoryResponse{Pin: v.pin, Mount: mount, Entries: page.Entries, Continuation: page.Continuation, Exhausted: page.Exhausted}, nil
}

// listDelivered enumerates one delivered directory of the composed tree. A
// directory inside a mount subtree pages that member's source directory and
// merges per-file entries into its first page; a directory that exists only
// through per-file entries, or that parents mount paths, lists those directly
// without paging. A name shared between a per-file entry and delivered source
// content is a conflict, never an order-based override
// (docs/reviewed/dataset.md, "冲突不得按挂载顺序覆盖").
func (v *fileView) listDelivered(request FileDirectoryRequest) (FileDirectoryResponse, error) {
	if !v.delivered {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered paths address the repository view; the semantic view has no per-file entries")
	}
	if request.MountPath != "" || request.Directory != "" {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered listing takes path alone; mountPath and directory address one mount")
	}
	dir := dataset.NormalizeMountPath(request.Path)
	owning, owns := v.mountOwning(dir)
	if owns {
		return v.listDeliveredMount(dir, owning, request)
	}
	return v.listDeliveredDerived(dir)
}

func (v *fileView) listDeliveredMount(dir string, mount dataset.VirtualMount, request FileDirectoryRequest) (FileDirectoryResponse, error) {
	relative := ""
	if dir != mount.Path {
		relative = strings.TrimPrefix(dir, mount.Path+"/")
	}
	page, err := v.readMountDirectory(mount, relative, request)
	if err != nil {
		return FileDirectoryResponse{}, err
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
				return FileDirectoryResponse{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"delivered path %s has conflicting sources", item.Target)
			}
			claimed[name] = struct{}{}
			entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: "file"})
		}
	}
	return FileDirectoryResponse{
		Pin: v.pin, Mount: mount, Entries: entries, Continuation: page.Continuation, Exhausted: page.Exhausted,
	}, nil
}

// listDeliveredDerived lists a delivered directory that no mount subtree
// pages: mount paths and per-file entries below it become direct entries.
// Mount carries the delivered coordinate only; no single member owns it.
func (v *fileView) listDeliveredDerived(dir string) (FileDirectoryResponse, error) {
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
				return FileDirectoryResponse{}, err
			}
		}
	}
	for _, item := range v.tree.FilesIn(dir) {
		if err := claim(path.Base(item.Target), "file"); err != nil {
			return FileDirectoryResponse{}, err
		}
	}
	for _, name := range v.tree.DirsIn(dir) {
		if err := claim(name, "directory"); err != nil {
			return FileDirectoryResponse{}, err
		}
	}
	entries := make([]snapshot.DirectoryEntry, 0, len(claimed))
	for name, kind := range claimed {
		entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: kind})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return FileDirectoryResponse{
		Pin: v.pin, Mount: dataset.VirtualMount{Path: dir}, Entries: entries, Exhausted: true,
	}, nil
}

func mountLabelFor(dir string) string {
	if dir == "" {
		return "<root>"
	}
	return dir
}

func (v *fileView) Read(request FileReadRequest) (FileReadResponse, error) {
	// Path explicitly addresses the delivered tree. A File with an empty
	// MountPath addresses the declared root mount instead; routing it through
	// delivered addressing would reject a valid root-relative file read.
	if request.Path != "" || (request.MountPath == "" && request.File == "") {
		return v.readDelivered(request)
	}
	mount, err := v.mount(request.MountPath)
	if err != nil {
		return FileReadResponse{}, err
	}
	if request.Offset < 0 || request.Length < 0 || request.Length > 4<<20 {
		return FileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "file range must use a non-negative offset and length no greater than 4 MiB")
	}
	length := request.Length
	if length == 0 {
		length = 512 << 10
	}
	content, readErr := v.readSource(mount, request.File)
	if v.record != nil {
		evidence := FileReadEvidence{Pin: v.pin, Repository: mount.Repository, Commit: mount.Commit, Path: path.Join(mount.Path, request.File), File: request.File, MountPath: mount.Path, Bytes: len(content), Err: readErr}
		if err := v.record(evidence); err != nil && readErr == nil {
			return FileReadResponse{}, err
		}
	}
	if readErr != nil {
		return FileReadResponse{}, readErr
	}
	start := request.Offset
	if start > int64(len(content)) {
		start = int64(len(content))
	}
	end := start + int64(length)
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return FileReadResponse{
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
func (v *fileView) readDelivered(request FileReadRequest) (FileReadResponse, error) {
	if !v.delivered {
		return FileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered paths address the repository view; the semantic view has no per-file entries")
	}
	if request.MountPath != "" || request.File != "" {
		return FileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered read takes path alone; mountPath and file address one mount")
	}
	if request.Offset < 0 || request.Length < 0 || request.Length > 4<<20 {
		return FileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "file range must use a non-negative offset and length no greater than 4 MiB")
	}
	delivered := dataset.NormalizeMountPath(request.Path)
	if delivered == "" {
		return FileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered file path is required")
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
		probePath, pathErr := RepositoryPath(owning.SubPath, relative)
		if pathErr == nil {
			// Readability probe only: a readable member file at the same
			// delivered path means conflicting sources. The probe content
			// itself is discarded.
			if _, probeErr := v.readMemberFile(owning.Repository, probePath, owning.Commit); probeErr == nil {
				return FileReadResponse{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"delivered path %s has conflicting sources", delivered)
			} else if kernel.CodeOf(probeErr) != kernel.ErrKnowledgeRefUnresolved {
				return FileReadResponse{}, probeErr
			}
		}
	}
	content, readErr := v.readMemberFile(item.Repository, item.File, item.Commit)
	mount := dataset.VirtualMount{
		Path:       deliveredParent(delivered),
		Repository: item.Repository,
		Commit:     item.Commit,
	}
	deliveredItem := item
	if err := v.recordDeliveredRead(delivered, item.Repository, item.Commit, content, readErr); err != nil && readErr == nil {
		return FileReadResponse{}, err
	}
	if readErr != nil {
		return FileReadResponse{}, readErr
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
	return FileReadResponse{
		Pin: v.pin, Mount: mount, File: item.File, Path: delivered, Item: &deliveredItem,
		Offset: start, TotalBytes: int64(len(content)), EOF: end == int64(len(content)),
		Content: content[start:end],
	}, nil
}

// readDeliveredMount serves a delivered path that belongs to a mount subtree
// by mapping it back to the member path, then reusing the mount-relative
// read. Response Path keeps the delivered coordinate the caller asked for.
func (v *fileView) readDeliveredMount(delivered string, request FileReadRequest) (FileReadResponse, error) {
	owning, owns := v.mountOwning(delivered)
	if !owns {
		return FileReadResponse{}, kernel.Fail(kernel.ErrForbidden, "delivered path %s is not present or not authorized", delivered)
	}
	relative := ""
	if delivered != owning.Path {
		relative = strings.TrimPrefix(delivered, owning.Path+"/")
	}
	mounted := FileReadRequest{
		FileCoordinate: request.FileCoordinate,
		MountPath:      owning.Path,
		File:           relative,
		Offset:         request.Offset,
		Length:         request.Length,
	}
	out, err := v.Read(mounted)
	if err != nil {
		return out, err
	}
	out.Path = delivered
	return out, nil
}

func (v *fileView) readMemberFile(repository kernel.RepositoryID, memberPath string, commit kernel.CommitID) ([]byte, error) {
	store, ok := v.sources.Get(repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", repository)
	}
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support fixed file reads", repository)
	}
	return tree.ReadFile(memberPath, commit)
}

func (v *fileView) recordDeliveredRead(delivered string, repository kernel.RepositoryID, commit kernel.CommitID, content []byte, readErr error) error {
	if v.record == nil {
		return nil
	}
	return v.record(FileReadEvidence{Pin: v.pin, Repository: repository, Commit: commit, Path: delivered, Bytes: len(content), Err: readErr})
}

func deliveredParent(delivered string) string {
	parent := path.Dir(delivered)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

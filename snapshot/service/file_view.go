package service

import (
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"path"
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
	pin         dataset.ResolvedKnowledgeSet
	mounts      []dataset.VirtualMount
	tree        dataset.DatasetDeliveredTree
	items       []dataset.DatasetItem
	sources     Sources
	delivered   bool
	directory   func(dataset.VirtualMount, string, int, string) (snapshot.DirectoryPage, error)
	sourceRead  func(dataset.VirtualMount, string) ([]byte, error)
	record      func(FileReadEvidence) error
	pagingScope kernel.Digest
}

func NewFileView(options ViewOptions) FileView {
	items := dataset.DatasetFileItems(options.Items)
	scope := append([]dataset.DatasetItem{}, options.Pin.Items...)
	if len(scope) == 0 { // Trusted programmatic views may predate pin item lists.
		for _, mount := range options.Mounts {
			scope = append(scope, dataset.DatasetItem{Kind: dataset.DatasetItemPrefix, Target: dataset.NormalizeMountPath(mount.Path), Prefix: dataset.NormalizeMountPath(mount.SubPath), Repository: mount.Repository, Commit: mount.Commit})
		}
		scope = append(scope, items...)
	}
	return &fileView{pin: options.Pin, mounts: append([]dataset.VirtualMount{}, options.Mounts...), tree: dataset.NewDatasetDeliveredTree(items), items: scope, sources: options.Sources, delivered: options.Delivered, directory: options.Directory, sourceRead: options.Read, record: options.Record, pagingScope: filePagingScope(options.Mounts, items, scope)}
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
	// Path selects delivered addressing. Directory with an empty MountPath
	// selects the declared root mount; no coordinates select delivered root.
	if request.Path != "" && (request.MountPath != "" || request.Directory != "") {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "directory addressing must use delivered path or mount-relative coordinates")
	}
	if request.MountPath == "" && request.Directory == "" {
		return v.listDelivered(request)
	}
	mount, err := v.mount(request.MountPath)
	if err != nil {
		return FileDirectoryResponse{}, err
	}
	if v.delivered {
		if _, err := RepositoryPath(mount.SubPath, request.Directory); err != nil {
			return FileDirectoryResponse{}, err
		}
		directory := path.Join(mount.Path, request.Directory)
		if directory == "." {
			directory = ""
		}
		if v.composedDirectory(directory, mount) {
			request.Path, request.MountPath, request.Directory = directory, "", ""
			return v.listDelivered(request)
		}
		if _, err := v.literalMountFor(directory); err != nil {
			return FileDirectoryResponse{}, err
		}
	}

	page, err := v.readMountDirectory(mount, request.Directory, request)
	if err != nil {
		return FileDirectoryResponse{}, err
	}
	return FileDirectoryResponse{Pin: v.pin, Mount: mount, Entries: page.Entries, Continuation: page.Continuation, Exhausted: page.Exhausted}, nil
}

// listDelivered enumerates one delivered directory of the composed tree. A
// directory pages derived children first, then its owning source's pages.
// A name shared between a per-file entry and delivered source content is a
// conflict, never an order-based override
// (doc/design/dataset.md, "冲突不得按挂载顺序覆盖").
func (v *fileView) listDelivered(request FileDirectoryRequest) (FileDirectoryResponse, error) {
	if !v.delivered {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered paths address the repository view; the semantic view has no per-file entries")
	}
	if request.MountPath != "" || request.Directory != "" {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "delivered listing takes path alone; mountPath and directory address one mount")
	}
	dir := dataset.NormalizeMountPath(request.Path)
	if request.Path != "" && !ValidRelativePath(request.Path) {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid delivered directory path")
	}
	return v.listDeliveredPage(dir, request)
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
	if v.delivered {
		if _, err := RepositoryPath(mount.SubPath, request.File); err != nil {
			return FileReadResponse{}, err
		}
		request.Path, request.MountPath, request.File = path.Join(mount.Path, request.File), "", ""
		return v.readDelivered(request)
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

// readDelivered compiles ownership from the complete fixed scope, then checks
// current visibility and old-list conflicts before reading selected bytes.
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
	item, err := v.literalRoute(request.Path)
	if err != nil {
		return FileReadResponse{}, err
	}
	if item.Kind == dataset.DatasetItemPrefix {
		if declared, visible := v.declaredDirectoryVisibility(delivered); declared {
			if !visible {
				return FileReadResponse{}, kernel.Fail(kernel.ErrForbidden, "delivered path is a hidden declared directory")
			}
			return FileReadResponse{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "delivered path is a directory")
		}
	}
	mount := dataset.VirtualMount{Path: deliveredParent(delivered), Repository: item.Repository, Commit: item.Commit}
	file := item.File
	var deliveredItem *dataset.DatasetItem
	var content []byte
	var readErr error
	if item.Kind == dataset.DatasetItemFile {
		deliveredItem = &item
		content, readErr = v.readMemberFile(item.Repository, item.File, item.Commit)
	} else {
		mount, _ = v.visiblePrefix(item)
		file = delivered
		if mount.Path != "" {
			file = strings.TrimPrefix(strings.TrimPrefix(delivered, mount.Path), "/")
		}
		content, readErr = v.readSource(mount, file)
	}
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
	// Length 0 uses the same 512 KiB default as mount-relative reads.
	length := request.Length
	if length == 0 {
		length = 512 << 10
	}
	end := start + int64(length)
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return FileReadResponse{
		Pin: v.pin, Mount: mount, File: file, Path: delivered, Item: deliveredItem,
		Offset: start, TotalBytes: int64(len(content)), EOF: end == int64(len(content)),
		Content: content[start:end],
	}, nil
}

func (v *fileView) readMemberFile(repository kernel.RepositoryID, memberPath string, commit kernel.CommitID) ([]byte, error) {
	store, ok := v.source(repository)
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

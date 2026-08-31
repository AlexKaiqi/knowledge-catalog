package cli

import (
	"encoding/json"
	"path"
	"sort"
	"strings"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

type workspaceFileCoordinate struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
}

type workspaceFileMountsRequest struct {
	workspaceFileCoordinate
}

type workspaceFileDirectoryRequest struct {
	workspaceFileCoordinate
	MountPath    string `json:"mountPath"`
	Directory    string `json:"directory,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type workspaceFileReadRequest struct {
	workspaceFileCoordinate
	MountPath string `json:"mountPath"`
	File      string `json:"file"`
	Offset    int64  `json:"offset,omitempty"`
	Length    int    `json:"length,omitempty"`
}

type workspaceFileMountsResponse struct {
	Pin    catalog.ResolvedWorkspace `json:"pin"`
	Mounts []catalog.VirtualMount    `json:"mounts"`
}

type workspaceFileDirectoryResponse struct {
	Pin          catalog.ResolvedWorkspace `json:"pin"`
	Mount        catalog.VirtualMount      `json:"mount"`
	Entries      []snapshot.DirectoryEntry `json:"entries"`
	Continuation string                    `json:"continuation,omitempty"`
	Exhausted    bool                      `json:"exhausted"`
}

type workspaceFileReadResponse struct {
	Pin        catalog.ResolvedWorkspace `json:"pin"`
	Mount      catalog.VirtualMount      `json:"mount"`
	File       string                    `json:"file"`
	Offset     int64                     `json:"offset"`
	TotalBytes int64                     `json:"totalBytes"`
	EOF        bool                      `json:"eof"`
	Content    []byte                    `json:"content"`
}

type workspaceFileView struct {
	home    string
	flags   map[string]FlagValue
	opened  *Home
	pin     catalog.ResolvedWorkspace
	mounts  []catalog.VirtualMount
	visible map[string]bool
}

func openWorkspaceFileView(home, principal string, coordinate workspaceFileCoordinate, requirePin bool, observe authorizationObserver) (*workspaceFileView, error) {
	if strings.TrimSpace(coordinate.Workspace) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "workspace is required")
	}
	if requirePin && len(coordinate.Pin) == 0 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "a fixed ResolvedWorkspace pin is required")
	}
	flags := compactFlags(map[string]FlagValue{
		"home": home, "as": principal, "catalog": coordinate.Catalog, "workspace": coordinate.Workspace,
	})
	if len(coordinate.Pin) > 0 {
		flags["pin"] = string(coordinate.Pin)
	}
	if err := authorize(home, "workspace.resolve", flags, observe); err != nil {
		return nil, err
	}
	opened, err := Open(home)
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(opened, flags)
	if err != nil {
		_ = opened.Close()
		return nil, err
	}
	definition, err := effectiveWorkspace(opened, home, cat, coordinate.Workspace, flags)
	if err != nil {
		_ = opened.Close()
		return nil, err
	}
	pin, err := resolveOrReplay(opened, home, cat, coordinate.Workspace, flags)
	if err != nil {
		_ = opened.Close()
		return nil, err
	}
	mounts, err := catalog.ListVirtualMountsAt(definition, pin)
	if err != nil {
		_ = opened.Close()
		return nil, err
	}
	visible := map[string]bool{}
	filtered := mounts[:0]
	for _, mount := range mounts {
		allowed, allowErr := workspaceFSMayReadRepository(home, flags, string(mount.Repository))
		if allowErr != nil {
			_ = opened.Close()
			return nil, allowErr
		}
		if allowed {
			visible[mount.Path] = true
			filtered = append(filtered, mount)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Path < filtered[j].Path })
	return &workspaceFileView{home: home, flags: flags, opened: opened, pin: pin, mounts: filtered, visible: visible}, nil
}

func (v *workspaceFileView) Close() { _ = v.opened.Close() }

func (v *workspaceFileView) mount(value string) (catalog.VirtualMount, error) {
	clean := strings.Trim(path.Clean("/"+value), "/")
	for _, mount := range v.mounts {
		if mount.Path == clean {
			return mount, nil
		}
	}
	return catalog.VirtualMount{}, kernel.Fail(kernel.ErrForbidden, "mount %s is not present or not authorized", clean)
}

func (v *workspaceFileView) list(request workspaceFileDirectoryRequest) (workspaceFileDirectoryResponse, error) {
	mount, err := v.mount(request.MountPath)
	if err != nil {
		return workspaceFileDirectoryResponse{}, err
	}
	directory, err := workspaceFSRepositoryPath(mount.SubPath, request.Directory)
	if err != nil {
		return workspaceFileDirectoryResponse{}, err
	}
	store, ok := v.opened.Store.Get(mount.Repository)
	if !ok {
		return workspaceFileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
	}
	reader, ok := store.(snapshot.DirectoryReader)
	if !ok {
		return workspaceFileDirectoryResponse{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not support directory paging", mount.Repository)
	}
	page, err := reader.ReadDirectory(snapshot.DirectoryRequest{
		Commit: mount.Commit, Directory: directory, Limit: request.Limit, Continuation: request.Continuation,
	})
	if err != nil {
		return workspaceFileDirectoryResponse{}, err
	}
	if page.Generation != string(mount.Commit) {
		return workspaceFileDirectoryResponse{}, kernel.Fail(kernel.ErrPreconditionFailed, "directory page generation moved from fixed pin")
	}
	return workspaceFileDirectoryResponse{Pin: v.pin, Mount: mount, Entries: page.Entries, Continuation: page.Continuation, Exhausted: page.Exhausted}, nil
}

func (v *workspaceFileView) read(request workspaceFileReadRequest) (workspaceFileReadResponse, error) {
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
	repositoryPath, err := workspaceFSRepositoryPath(mount.SubPath, request.File)
	if err != nil {
		return workspaceFileReadResponse{}, err
	}
	store, ok := v.opened.Store.Get(mount.Repository)
	if !ok {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", mount.Repository)
	}
	tree, ok := snapshot.TreeStoreOf(store)
	if !ok {
		return workspaceFileReadResponse{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support fixed file reads", mount.Repository)
	}
	content, readErr := tree.ReadFile(repositoryPath, mount.Commit)
	result := map[string]any{"mountPath": mount.Path, "file": request.File, "repository": mount.Repository, "commit": mount.Commit}
	readFlags := make(map[string]FlagValue, len(v.flags)+2)
	for name, value := range v.flags {
		readFlags[name] = value
	}
	readFlags["repo"] = string(mount.Repository)
	readFlags["path"] = path.Join(mount.Path, request.File)
	readFlags["_action"] = "file.read"
	if _, accessErr := recordKnowledgeAccess(v.home, "file-read", readFlags, result, readErr); accessErr != nil && readErr == nil {
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

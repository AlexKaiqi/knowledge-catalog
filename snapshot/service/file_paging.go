package service

import (
	"encoding/base64"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

const defaultDeliveredPageSize = 500

// A delivered position never supplies entries or authority. Every request
// reconstructs the visible tree and, when needed, replays the source's own
// directory position against the accepted commit.
type deliveredDirectoryCursor struct {
	Version   int           `json:"version"`
	Dataset   string        `json:"dataset"`
	Revision  int           `json:"revision"`
	PinID     string        `json:"pinId"`
	Directory string        `json:"directory"`
	Scope     kernel.Digest `json:"scope"`
	Derived   int           `json:"derived"`
	Source    string        `json:"source,omitempty"`
	Check     kernel.Digest `json:"check"`
}

func filePagingScope(mounts []dataset.VirtualMount, items, scope []dataset.DatasetItem) kernel.Digest {
	orderedMounts := append([]dataset.VirtualMount{}, mounts...)
	orderedItems := append([]dataset.DatasetItem{}, items...)
	orderedScope := append([]dataset.DatasetItem{}, scope...)
	sort.Slice(orderedMounts, func(i, j int) bool { return orderedMounts[i].Path < orderedMounts[j].Path })
	sort.Slice(orderedItems, func(i, j int) bool { return orderedItems[i].Target < orderedItems[j].Target })
	sort.Slice(orderedScope, func(i, j int) bool { return orderedScope[i].Target < orderedScope[j].Target })
	return kernel.CanonicalDigest(struct {
		Mounts []dataset.VirtualMount
		Items  []dataset.DatasetItem
		Scope  []dataset.DatasetItem
	}{orderedMounts, orderedItems, orderedScope})
}

func deliveredCursorCheck(cursor deliveredDirectoryCursor) kernel.Digest {
	cursor.Check = ""
	return kernel.CanonicalDigest(cursor)
}

func (v *fileView) deliveredCursor(raw, directory string, count int) (deliveredDirectoryCursor, error) {
	want := deliveredDirectoryCursor{Version: 1, Dataset: v.pin.SetID, Revision: v.pin.Revision, PinID: v.pin.PinID, Directory: directory, Scope: v.pagingScope}
	if raw == "" {
		return want, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	var cursor deliveredDirectoryCursor
	if err != nil || json.Unmarshal(decoded, &cursor) != nil || !v.deliveredCursorBasis(cursor, directory) || cursor.Check != deliveredCursorCheck(cursor) {
		return want, kernel.Fail(kernel.ErrUsageInvalid, "delivered directory continuation does not match its fixed Dataset and directory")
	}
	if cursor.Scope != want.Scope {
		return want, kernel.Fail(kernel.ErrPreconditionFailed, "visible file scope changed; restart directory paging")
	}
	if cursor.Derived < 0 || cursor.Derived > count || (cursor.Source != "" && cursor.Derived != count) {
		return want, kernel.Fail(kernel.ErrUsageInvalid, "delivered directory continuation has an invalid position")
	}
	return cursor, nil
}

func (v *fileView) deliveredCursorBasis(cursor deliveredDirectoryCursor, directory string) bool {
	return cursor.Version == 1 && cursor.Dataset == v.pin.SetID && cursor.Revision == v.pin.Revision && cursor.PinID == v.pin.PinID && cursor.Directory == directory
}

func encodeDeliveredCursor(cursor deliveredDirectoryCursor) string {
	cursor.Check = deliveredCursorCheck(cursor)
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func (v *fileView) listDeliveredPage(directory string, request FileDirectoryRequest) (FileDirectoryResponse, error) {
	if err := v.rejectFileDirectory(directory); err != nil {
		return FileDirectoryResponse{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultDeliveredPageSize
	}
	if limit < 1 {
		return FileDirectoryResponse{}, kernel.Fail(kernel.ErrUsageInvalid, "directory limit must be non-negative")
	}
	derived, err := v.deliveredChildren(directory)
	if err != nil {
		return FileDirectoryResponse{}, err
	}
	cursor, err := v.deliveredCursor(request.Continuation, directory, len(derived))
	if err != nil {
		return FileDirectoryResponse{}, err
	}
	end := cursor.Derived + min(limit, len(derived)-cursor.Derived)
	entries := append([]snapshot.DirectoryEntry{}, derived[cursor.Derived:end]...)
	cursor.Derived = end
	mount, owns := v.completeMountOwning(directory)
	if !owns {
		mount = dataset.VirtualMount{Path: directory}
	}
	exhausted := end == len(derived) && !owns
	if end == len(derived) && owns && len(entries) < limit {
		source, err := v.deliveredSourcePage(directory, mount, request, cursor.Source, limit-len(entries), derived)
		if err != nil {
			return FileDirectoryResponse{}, err
		}
		entries = append(entries, source.Entries...)
		cursor.Source, exhausted = source.Continuation, source.Exhausted
	}
	response := FileDirectoryResponse{Pin: v.pin, Mount: mount, Entries: entries, Exhausted: exhausted}
	if !exhausted {
		response.Continuation = encodeDeliveredCursor(cursor)
	}
	return response, nil
}

func (v *fileView) deliveredChildren(directory string) ([]snapshot.DirectoryEntry, error) {
	claimed := map[string]string{}
	claim := func(name, kind string) error {
		if prior, exists := claimed[name]; exists && prior != kind {
			return kernel.Fail(kernel.ErrDatasetInvalid, "delivered path %s/%s has conflicting sources", mountLabelFor(directory), name)
		}
		claimed[name] = kind
		return nil
	}
	for _, mount := range v.mounts {
		relative := mount.Path
		if directory != "" {
			var below bool
			relative, below = strings.CutPrefix(mount.Path, directory+"/")
			if !below {
				continue
			}
		}
		if relative != "" {
			if err := claim(strings.Split(relative, "/")[0], "directory"); err != nil {
				return nil, err
			}
		}
	}
	for _, item := range v.tree.FilesIn(directory) {
		if err := claim(path.Base(item.Target), "file"); err != nil {
			return nil, err
		}
	}
	for _, name := range v.tree.DirsIn(directory) {
		if err := claim(name, "directory"); err != nil {
			return nil, err
		}
	}
	entries := make([]snapshot.DirectoryEntry, 0, len(claimed))
	for name, kind := range claimed {
		entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: kind})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (v *fileView) deliveredSourcePage(directory string, mount dataset.VirtualMount, request FileDirectoryRequest, continuation string, limit int, derived []snapshot.DirectoryEntry) (snapshot.DirectoryPage, error) {
	relative := ""
	if directory != mount.Path {
		relative = strings.TrimPrefix(directory, mount.Path+"/")
	}
	request.Limit, request.Continuation = limit, continuation
	page, err := v.readMountDirectory(mount, relative, request)
	if err != nil {
		return page, err
	}
	if page.Generation != string(mount.Commit) || len(page.Entries) > limit ||
		(!page.Exhausted && (page.Continuation == "" || page.Continuation == continuation)) {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "source directory page lost its fixed basis, limit or progress")
	}
	page.Entries, err = v.filterSourceChildren(page.Entries, directory, mount)
	if err == nil {
		page.Entries, err = mergeDeliveredSourceEntries(page.Entries, derived, directory)
	}
	return page, err
}

func mergeDeliveredSourceEntries(source, derived []snapshot.DirectoryEntry, directory string) ([]snapshot.DirectoryEntry, error) {
	known := map[string]string{}
	for _, entry := range derived {
		known[entry.Name] = entry.Kind
	}
	entries := []snapshot.DirectoryEntry{}
	for _, entry := range source {
		if kind, exists := known[entry.Name]; exists {
			if kind != "directory" || entry.Kind != "directory" {
				return nil, kernel.Fail(kernel.ErrDatasetInvalid, "delivered path %s/%s has conflicting sources", mountLabelFor(directory), entry.Name)
			}
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

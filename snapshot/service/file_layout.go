package service

import (
	"strings"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func (v *fileView) source(repository kernel.RepositoryID) (snapshot.Store, bool) {
	if v.sources == nil {
		return nil, false
	}
	return v.sources.Get(repository)
}

func (v *fileView) visibleFile(item dataset.DatasetItem) bool {
	visible, ok := v.tree.Find(item.Target)
	return ok && visible.Repository == item.Repository && visible.Commit == item.Commit && visible.File == item.File
}

func (v *fileView) visiblePrefix(item dataset.DatasetItem) (dataset.VirtualMount, bool) {
	for _, mount := range v.mounts {
		if mount.Repository == item.Repository && mount.Commit == item.Commit && mount.Path == dataset.NormalizeMountPath(item.Target) && dataset.NormalizeMountPath(mount.SubPath) == item.Prefix {
			return mount, true
		}
	}
	return dataset.VirtualMount{}, false
}

// A declared mount or selected descendant makes this path a directory even
// when current permissions hide the item that owns it.
func (v *fileView) declaredDirectoryVisibility(target string) (declared, visible bool) {
	for _, item := range v.items {
		if !strings.HasPrefix(item.Target, target+"/") && (item.Kind != dataset.DatasetItemPrefix || item.Target != target) {
			continue
		}
		declared = true
		if item.Kind == dataset.DatasetItemFile {
			visible = visible || v.visibleFile(item)
		} else {
			_, shown := v.visiblePrefix(item)
			visible = visible || shown
		}
	}
	return declared, visible
}

func (v *fileView) literalRoute(delivered string) (dataset.DatasetItem, error) {
	if !ValidRelativePath(delivered) {
		return dataset.DatasetItem{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid delivered file path")
	}
	for _, item := range v.items {
		if item.Kind == dataset.DatasetItemFile && strings.HasPrefix(delivered, item.Target+"/") {
			return dataset.DatasetItem{}, kernel.Fail(kernel.ErrForbidden, "delivered path is below a selected file")
		}
	}
	item, err := dataset.RouteDatasetFile(v.items, delivered)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return item, kernel.Fail(kernel.ErrForbidden, "delivered path is not present or not authorized")
		}
		return item, err
	}
	visible := v.visibleFile(item)
	if item.Kind == dataset.DatasetItemPrefix {
		_, visible = v.visiblePrefix(item)
	}
	if !visible {
		return dataset.DatasetItem{}, kernel.Fail(kernel.ErrForbidden, "delivered path is not present or not authorized")
	}
	// Ancestors of selected file entries are directories in the composed tree;
	// an old invalid pin must not expose a source file at that directory path.
	for _, file := range v.items {
		if file.Kind == dataset.DatasetItemFile && (file.Target == delivered || strings.HasPrefix(file.Target, delivered+"/")) {
			if !v.visibleFile(file) {
				if file.Target != delivered {
					return dataset.DatasetItem{}, kernel.Fail(kernel.ErrForbidden, "delivered path contains a hidden file selection")
				}
				continue
			}
			if err := v.checkFileConflict(file.Target); err != nil {
				return dataset.DatasetItem{}, err
			}
		}
	}
	return item, nil
}

func (v *fileView) checkFileConflict(target string) error {
	prefixes := []dataset.DatasetItem{}
	for _, item := range v.items {
		if item.Kind == dataset.DatasetItemPrefix {
			prefixes = append(prefixes, item)
		}
	}
	if prefix, err := dataset.RouteDatasetFile(prefixes, target); err == nil {
		if _, visible := v.visiblePrefix(prefix); !visible {
			return kernel.Fail(kernel.ErrForbidden, "file layout depends on a source outside the authorized scope")
		}
	}
	return dataset.CheckDatasetFileConflict(v.items, target, v.source)
}

func (v *fileView) literalMountFor(directory string) (dataset.VirtualMount, error) {
	item, err := v.literalRoute(directory)
	if err != nil {
		return dataset.VirtualMount{}, err
	}
	if item.Kind == dataset.DatasetItemFile {
		return dataset.VirtualMount{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "delivered path is a file")
	}
	mount, _ := v.visiblePrefix(item)
	return mount, nil
}

func (v *fileView) composedDirectory(directory string, mount dataset.VirtualMount) bool {
	for _, item := range v.items {
		if item.Kind == dataset.DatasetItemFile {
			if directory == "" || item.Target == directory || strings.HasPrefix(item.Target, directory+"/") {
				return true
			}
		} else if item.Target != mount.Path && item.Target != "" && (directory == "" || item.Target == directory || strings.HasPrefix(item.Target, directory+"/")) {
			return true
		}
	}
	return false
}

func (v *fileView) rejectFileDirectory(directory string) error {
	for _, item := range v.items {
		if item.Kind != dataset.DatasetItemFile || (directory != item.Target && !strings.HasPrefix(directory, item.Target+"/")) {
			continue
		}
		if !v.visibleFile(item) {
			return kernel.Fail(kernel.ErrForbidden, "directory is a hidden file selection")
		}
		if err := v.checkFileConflict(item.Target); err != nil {
			return err
		}
		return kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "delivered path is a file")
	}
	return nil
}

func (v *fileView) completeMountOwning(directory string) (dataset.VirtualMount, bool) {
	if directory == "" {
		for _, item := range v.items {
			if item.Kind == dataset.DatasetItemPrefix && item.Target == "" {
				return v.visiblePrefix(item)
			}
		}
		return dataset.VirtualMount{}, false
	}
	item, err := dataset.RouteDatasetFile(v.items, directory)
	if err != nil || item.Kind != dataset.DatasetItemPrefix {
		return dataset.VirtualMount{}, false
	}
	return v.visiblePrefix(item)
}

func (v *fileView) filterSourceChildren(source []snapshot.DirectoryEntry, directory string, mount dataset.VirtualMount) ([]snapshot.DirectoryEntry, error) {
	out := []snapshot.DirectoryEntry{}
	seen := map[string]bool{}
	for _, entry := range source {
		if !ValidRelativePath(entry.Name) || strings.Contains(entry.Name, "/") ||
			(entry.Kind != "file" && entry.Kind != "directory") || seen[entry.Name] {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "source directory returned an invalid child")
		}
		seen[entry.Name] = true
		target := entry.Name
		if directory != "" {
			target = directory + "/" + entry.Name
		}
		if declared, visible := v.declaredDirectoryVisibility(target); declared && entry.Kind == "file" {
			if !visible {
				continue
			}
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "delivered path %s is a declared directory with source file content", target)
		}
		item, err := dataset.RouteDatasetFile(v.items, target)
		if err != nil {
			return nil, err
		}
		if item.Kind == dataset.DatasetItemFile {
			if !v.visibleFile(item) || (mount.Path == "" && entry.Kind == "file") {
				continue
			}
		} else if item.Target != mount.Path || item.Repository != mount.Repository || item.Commit != mount.Commit {
			continue // A declared mount masks fallback content even when hidden.
		}
		out = append(out, entry)
	}
	return out, nil
}

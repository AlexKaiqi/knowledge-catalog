package dataset

import (
	"path"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// Publication probes only explicitly selected files and the paths that could
// collide with them. Disjoint prefix layouts never require tree enumeration.
func (c *service) validatePublicationFiles(items []DatasetItem) error {
	if err := c.validateRootPrefixDirectories(items); err != nil {
		return err
	}
	for _, item := range items {
		if item.Kind != DatasetItemFile {
			continue
		}
		if err := c.validateSelectedFile(item); err != nil {
			return err
		}
		if err := CheckDatasetFileConflict(items, item.Target, c.store.Get); err != nil {
			return err
		}
	}
	return nil
}

func (c *service) validateRootPrefixDirectories(items []DatasetItem) error {
	var root DatasetItem
	for _, item := range items {
		if item.Kind == DatasetItemPrefix && normalizeMountPath(item.Target) == "" {
			root = item
			break
		}
	}
	if root.Kind == "" {
		return nil
	}
	for _, item := range items {
		if item.Kind != DatasetItemPrefix || normalizeMountPath(item.Target) == "" {
			continue
		}
		reader, err := c.fileReader(root)
		if err != nil {
			return err
		}
		if err := checkFileAncestors(reader, joinSubPath(root.Prefix, item.Target), root.Prefix, item.Target, false, root.Commit); err != nil {
			return err
		}
	}
	return nil
}

func (c *service) fileReader(item DatasetItem) (snapshot.TreeReader, error) {
	if c.store == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "service has no snapshot store")
	}
	return fixedFileReader(item, c.store.Get)
}

func fixedFileReader(item DatasetItem, source func(kernel.RepositoryID) (snapshot.Store, bool)) (snapshot.TreeReader, error) {
	if source == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset file conflict check requires source access")
	}
	store, ok := source(item.Repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not attached", item.Repository)
	}
	if store.Archived() {
		return nil, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", item.Repository)
	}
	reader, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support raw path reads", item.Repository)
	}
	return reader, nil
}

func (c *service) validateSelectedFile(item DatasetItem) error {
	reader, err := c.fileReader(item)
	if err != nil {
		return err
	}
	_, err = reader.ReadFile(item.File, item.Commit)
	return err
}

// CheckDatasetFileConflict validates only the explicit file at deliveredPath
// against its owning prefix's immutable content. It shares publication's
// bounded path/directory probes with consumers of previously accepted lists.
func CheckDatasetFileConflict(items []DatasetItem, deliveredPath string, source func(kernel.RepositoryID) (snapshot.Store, bool)) error {
	target := normalizeMountPath(deliveredPath)
	for _, item := range items {
		if item.Kind == DatasetItemFile && normalizeMountPath(item.Target) == target {
			if prefix, file, found := deliveredPrefixSource(items, target); found {
				return checkPrefixFileConflict(prefix, file, target, source)
			}
			break
		}
	}
	return nil
}

func checkPrefixFileConflict(prefix DatasetItem, file, target string, source func(kernel.RepositoryID) (snapshot.Store, bool)) error {
	reader, err := fixedFileReader(prefix, source)
	if err != nil {
		return err
	}
	if err := checkFileAncestors(reader, file, prefix.Prefix, target, normalizeMountPath(prefix.Target) == "", prefix.Commit); err != nil {
		return err
	}
	store, _ := source(prefix.Repository)
	directory, ok := snapshot.DirectoryReaderOf(store)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s requires directory paging to validate delivered path %s", prefix.Repository, target)
	}
	page, err := directory.ReadDirectory(snapshot.DirectoryRequest{Commit: prefix.Commit, Directory: file, Limit: 1})
	if err != nil && kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return err
	}
	if err != nil {
		return nil
	}
	if page.Generation != string(prefix.Commit) || len(page.Entries) > 1 || (len(page.Entries) == 0 && (!page.Exhausted || page.Continuation != "")) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "directory probe did not prove the fixed delivered path %s", target)
	}
	if len(page.Entries) > 0 {
		return deliveredConflict(target)
	}
	return nil
}

func checkFileAncestors(reader snapshot.TreeReader, file, prefix, target string, skipExact bool, commit kernel.CommitID) error {
	for probe := file; probe != "" && probe != "."; probe = path.Dir(probe) {
		if probe == file && skipExact {
			continue // An explicit file owns its exact slot before root fallback.
		}
		if _, err := reader.ReadFile(probe, commit); err == nil {
			return deliveredConflict(target)
		} else if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return err
		}
		if probe == prefix {
			break
		}
	}
	return nil
}

func deliveredConflict(target string) error {
	return kernel.Fail(kernel.ErrDatasetInvalid, "delivered path %s has conflicting sources", target)
}

// The root prefix is fallback ownership. A declared non-root prefix excludes
// the root at that path even when the two source trees contain the same names.
func deliveredPrefixSource(items []DatasetItem, target string) (DatasetItem, string, bool) {
	target = normalizeMountPath(target)
	var best DatasetItem
	found := false
	for _, item := range items {
		if item.Kind != DatasetItemPrefix {
			continue
		}
		mount := normalizeMountPath(item.Target)
		if mount != "" && target != mount && !strings.HasPrefix(target, mount+"/") {
			continue
		}
		if !found || len(mount) > len(normalizeMountPath(best.Target)) {
			best, found = item, true
		}
	}
	if !found {
		return DatasetItem{}, "", false
	}
	relative := target
	if mount := normalizeMountPath(best.Target); mount != "" {
		relative = strings.TrimPrefix(strings.TrimPrefix(target, mount), "/")
	}
	return best, joinSubPath(best.Prefix, relative), true
}

// RouteDatasetFile compiles one delivered path into frozen source coordinates.
// An exact file entry owns its slot; otherwise the declared prefix owns the
// path, falling back to root. Prefix results retain the original item and set
// File to its repository-relative path. The helper does not read a source or
// prove that the file exists; publication owns conflict and availability checks.
func RouteDatasetFile(items []DatasetItem, deliveredPath string) (DatasetItem, error) {
	if err := validateRelativeTreePath("delivered file path", deliveredPath, false); err != nil {
		return DatasetItem{}, kernel.Fail(kernel.ErrUsageInvalid, "%v", err)
	}
	target := normalizeMountPath(deliveredPath)
	for _, item := range items {
		if item.Kind == DatasetItemFile && normalizeMountPath(item.Target) == target {
			return item, nil
		}
	}
	if prefix, file, found := deliveredPrefixSource(items, target); found {
		prefix.File = file
		return prefix, nil
	}
	return DatasetItem{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "delivered file path %s is not selected", deliveredPath)
}

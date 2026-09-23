package catalog

import (
	"strings"

	"kc/kernel"
)

const (
	DatasetItemPrefix = "prefix"
	DatasetItemFile   = "file"
)

// DatasetItem is one published file pointer. Catalog stores opaque paths; it
// does not interpret object_id or Aspect. Target is unique inside one Dataset.
type DatasetItem struct {
	Target     string              `json:"target"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Kind       string              `json:"kind"`
	Prefix     string              `json:"prefix,omitempty"`
	File       string              `json:"file,omitempty"`
}

func (c *Catalog) freezeSources(sources []KnowledgeSetSource) ([]KnowledgeSetSource, error) {
	frozen := make([]KnowledgeSetSource, len(sources))
	copy(frozen, sources)
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	for i, src := range frozen {
		if existing, ok := commits[src.Repository]; ok {
			frozen[i].Commit = existing
			continue
		}
		if c.store == nil {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "catalog cannot freeze selectors without a snapshot store")
		}
		repo, ok := c.store.Get(src.Repository)
		if !ok {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s is not attached", src.Repository)
		}
		if src.Commit != "" {
			if !repo.HasCommit(src.Commit) {
				return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist in %s", src.Commit, src.Repository)
			}
			commits[src.Repository] = src.Commit
			continue
		}
		commit, ok := repo.GetRef(src.Selector)
		if !ok {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s has no ref %s", src.Repository, src.Selector)
		}
		if src.BaseRev != "" && commit != kernel.CommitID(src.BaseRev) {
			return nil, kernel.Fail(kernel.ErrNonFastForward,
				"repository %s selector %s is at %s, recipe baseRev is %s", src.Repository, src.Selector, commit, src.BaseRev)
		}
		frozen[i].Commit = commit
		commits[src.Repository] = commit
	}
	return frozen, nil
}

func datasetItemsFromSources(sources []KnowledgeSetSource, commits map[kernel.RepositoryID]kernel.CommitID) ([]DatasetItem, error) {
	items := make([]DatasetItem, 0, len(sources))
	seen := map[string]struct{}{}
	for _, src := range sources {
		item, err := datasetItemFromSource(src, commits)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[item.Target]; dup {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset target %s is not unique", itemTargetLabel(item.Target))
		}
		seen[item.Target] = struct{}{}
		items = append(items, item)
	}
	if err := validateDeliveredTargets(items); err != nil {
		return nil, err
	}
	return items, nil
}

func datasetItemFromSource(src KnowledgeSetSource, commits map[kernel.RepositoryID]kernel.CommitID) (DatasetItem, error) {
	item := DatasetItem{Repository: src.Repository, Commit: src.Commit}
	if commit, ok := commits[src.Repository]; ok {
		item.Commit = commit
	}
	if item.Commit == "" {
		return DatasetItem{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset item %s has no frozen commit", itemTargetLabel(src.Target))
	}
	if src.IsFileEntry() {
		if err := validateRelativeTreePath("dataset file", src.File, false); err != nil {
			return DatasetItem{}, err
		}
		if err := validateRelativeTreePath("delivered target", src.Target, false); err != nil {
			return DatasetItem{}, err
		}
		file := normalizeMemberSubPath(src.File)
		target := normalizeMountPath(src.Target)
		if file == "" || target == "" {
			return DatasetItem{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
				"file entry of repository %s needs a repository file and a delivered target", src.Repository)
		}
		item.Kind = DatasetItemFile
		item.File = file
		item.Target = target
		return item, nil
	}
	item.Kind = DatasetItemPrefix
	item.Prefix = normalizeMemberSubPath(src.SubPath)
	if src.Path != nil {
		item.Target = normalizeMountPath(*src.Path)
	} else {
		item.Target = string(src.Repository)
		if item.Prefix != "" {
			item.Target = string(src.Repository) + "/" + item.Prefix
		}
	}
	return item, nil
}

// validateDeliveredTargets enforces docs/reviewed/dataset.md's target rules
// across item kinds. Prefix items deliver whole directories; file items
// deliver one file each. A file may land inside a delivered directory
// (mixing sources into one organized directory), but it must not take a
// delivered directory's own place or be an ancestor of one: those layouts
// have no single unambiguous source.
func validateDeliveredTargets(items []DatasetItem) error {
	for _, file := range items {
		if file.Kind != DatasetItemFile {
			continue
		}
		for _, other := range items {
			if other.Kind == DatasetItemFile {
				// A delivered file occupies exactly one path: neither side of
				// a file pair may be an ancestor of the other. Exact duplicate
				// targets are already rejected by target uniqueness.
				if other.Target == file.Target {
					continue
				}
				if strings.HasPrefix(other.Target, file.Target+"/") || strings.HasPrefix(file.Target, other.Target+"/") {
					return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
						"delivered files %s and %s nest inside each other; a file entry delivers exactly one path",
						file.Target, other.Target)
				}
				continue
			}
			if other.Kind != DatasetItemPrefix {
				continue
			}
			if file.Target == other.Target {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"delivered path %s is a directory delivered from %s and cannot also be one file",
					file.Target, other.Repository)
			}
			if strings.HasPrefix(other.Target, file.Target+"/") {
				return kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"delivered directory %s (%s) cannot live under delivered file %s",
					other.Target, other.Repository, file.Target)
			}
		}
	}
	return nil
}

func itemTargetLabel(target string) string {
	if target == "" {
		return "<root>"
	}
	return target
}

// DatasetPathAllowed reports whether a repository path is in the published
// file list. An empty list is not a whole-repository alias.
func DatasetPathAllowed(items []DatasetItem, repository kernel.RepositoryID, path string) bool {
	for _, item := range items {
		if item.Repository != repository {
			continue
		}
		if datasetItemCovers(item, path) {
			return true
		}
	}
	return false
}

func datasetItemCovers(item DatasetItem, path string) bool {
	path = strings.Trim(path, "/")
	switch item.Kind {
	case DatasetItemFile:
		return path == strings.Trim(item.File, "/")
	case DatasetItemPrefix:
		prefix := strings.Trim(item.Prefix, "/")
		if prefix == "" {
			return true
		}
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return false
}

func datasetRestrictsRepository(items []DatasetItem, repository kernel.RepositoryID) bool {
	found := false
	for _, item := range items {
		if item.Repository != repository {
			continue
		}
		found = true
		if item.Kind == DatasetItemPrefix && strings.Trim(item.Prefix, "/") == "" {
			return false
		}
	}
	return found
}

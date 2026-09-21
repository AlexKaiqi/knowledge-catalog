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
		item := DatasetItem{
			Repository: src.Repository,
			Commit:     src.Commit,
			Kind:       DatasetItemPrefix,
			Prefix:     normalizeMemberSubPath(src.SubPath),
		}
		if commit, ok := commits[src.Repository]; ok {
			item.Commit = commit
		}
		if item.Commit == "" {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset item %s has no frozen commit", itemTargetLabel(item.Target))
		}
		if src.Path != nil {
			item.Target = normalizeMountPath(*src.Path)
		} else {
			item.Target = string(src.Repository)
			if item.Prefix != "" {
				item.Target = string(src.Repository) + "/" + item.Prefix
			}
		}
		if _, dup := seen[item.Target]; dup {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset target %s is not unique", itemTargetLabel(item.Target))
		}
		seen[item.Target] = struct{}{}
		items = append(items, item)
	}
	return items, nil
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

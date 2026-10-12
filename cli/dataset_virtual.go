package cli

import (
	"kc/kernel"
	"kc/snapshot/dataset"
)

// filterVirtualWorkspace narrows a resolved virtual tree to repositories the
// caller may dereference. Authorization is evaluated once per repository even
// when the same repository is mounted at several paths.
func filterVirtualWorkspace(def dataset.KnowledgeSet, resolved dataset.ResolvedKnowledgeSet, mounts []dataset.VirtualMount, mayRead func(kernel.RepositoryID) (bool, error)) (dataset.KnowledgeSet, dataset.ResolvedKnowledgeSet, []dataset.VirtualMount, error) {
	allowed := map[kernel.RepositoryID]bool{}
	visibleMounts := make([]dataset.VirtualMount, 0, len(mounts))
	for _, mount := range mounts {
		ok, known := allowed[mount.Repository]
		if !known {
			var err error
			ok, err = mayRead(mount.Repository)
			if err != nil {
				return dataset.KnowledgeSet{}, dataset.ResolvedKnowledgeSet{}, nil, err
			}
			allowed[mount.Repository] = ok
		}
		if ok {
			visibleMounts = append(visibleMounts, mount)
		}
	}

	visibleDef := def
	visibleDef.Sources = make([]dataset.KnowledgeSetSource, 0, len(def.Sources))
	visibleResolved := resolved
	visibleResolved.Repositories = map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		if !allowed[src.Repository] {
			continue
		}
		visibleDef.Sources = append(visibleDef.Sources, src)
		if commit, ok := resolved.Repositories[src.Repository]; ok {
			visibleResolved.Repositories[src.Repository] = commit
		}
	}
	return visibleDef, visibleResolved, visibleMounts, nil
}

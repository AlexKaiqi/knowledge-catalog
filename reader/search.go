package reader

import (
	"kc/kernel"
	"kc/repository"
)

// Search is whole-JSON contains on a pinned commit. Not production retrieval.
// Production: Projection.Build then hydrate Canonical (T8 / design 7.7).

func (r *Reader) Search(query string, repositoryID kernel.RepositoryID, commitID kernel.CommitID) (hits []repository.KnowledgeValue, err error) {
	defer func() {
		err = r.note("search", map[string]any{"repositoryId": string(repositoryID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return nil, err
	}
	return repo.Search(query, commitID)
}

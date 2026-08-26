package reader

import (
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

// Search is whole-JSON contains on a pinned commit. Not production retrieval.
// Production search is orchestrated by index.Index, which locates candidates
// and hydrates Canonical values through this layer's contracts (T8 / design 7.7).

func (r *Reader) Search(query string, repositoryID kernel.RepositoryID, commitID kernel.CommitID) (hits []knowledge.KnowledgeValue, err error) {
	defer func() {
		err = r.note("search", map[string]any{"repositoryId": string(repositoryID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return nil, err
	}
	listed, err := repo.List(commitID)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	for _, value := range listed {
		body, _ := json.Marshal(value.Value)
		if strings.Contains(strings.ToLower(string(body)), needle) {
			hits = append(hits, value)
		}
	}
	return hits, nil
}

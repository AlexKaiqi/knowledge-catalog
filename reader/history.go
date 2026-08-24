package reader

import (
	"kc/kernel"
	"kc/repository"
)

// LOG / DIFF / GET_PROVENANCE are three questions (design 7.5).
// They are not git log, not an ORIGIN crawl, and not interchangeable.

func (r *Reader) Log(repositoryID kernel.RepositoryID, objectID kernel.ObjectID, commitID kernel.CommitID, limit int) (revs []repository.ObjectRevision, err error) {
	defer func() {
		err = r.note("log", map[string]any{"repositoryId": string(repositoryID), "object": string(objectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return nil, err
	}
	return repo.Log(objectID, commitID, limit)
}

func (r *Reader) Diff(repositoryID kernel.RepositoryID, objectID kernel.ObjectID, from, to kernel.CommitID) (diff repository.ObjectDiff, err error) {
	defer func() {
		err = r.note("diff", map[string]any{"repositoryId": string(repositoryID), "object": string(objectID), "from": string(from), "to": string(to)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return repository.ObjectDiff{}, err
	}
	return repo.Diff(objectID, from, to)
}

func (r *Reader) GetProvenance(ref kernel.KnowledgeRef, commitID kernel.CommitID) (trace repository.ProvenanceTrace, err error) {
	defer func() {
		err = r.note("provenance", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return repository.ProvenanceTrace{}, err
	}
	return repo.GetProvenance(ref.Object, commitID)
}

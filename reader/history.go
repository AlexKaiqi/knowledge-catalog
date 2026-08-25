package reader

import (
	"kc/kernel"
	"kc/knowledge"
)

// LOG / DIFF / GET_PROVENANCE are three questions (design 7.5).
// They are not git log, not an ORIGIN crawl, and not interchangeable.

func (r *Reader) Log(repositoryID kernel.RepositoryID, objectID knowledge.ObjectID, commitID kernel.CommitID, limit int) (revs []knowledge.ObjectRevision, err error) {
	defer func() {
		err = r.note("log", map[string]any{"repositoryId": string(repositoryID), "object": string(objectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return nil, err
	}
	return repo.Log(objectID, commitID, limit)
}

func (r *Reader) Diff(repositoryID kernel.RepositoryID, objectID knowledge.ObjectID, from, to kernel.CommitID) (diff knowledge.ObjectDiff, err error) {
	defer func() {
		err = r.note("diff", map[string]any{"repositoryId": string(repositoryID), "object": string(objectID), "from": string(from), "to": string(to)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	return repo.Diff(objectID, from, to)
}

func (r *Reader) GetProvenance(ref knowledge.KnowledgeRef, commitID kernel.CommitID) (trace knowledge.ProvenanceTrace, err error) {
	defer func() {
		err = r.note("provenance", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	return repo.GetProvenance(ref.Object, commitID)
}

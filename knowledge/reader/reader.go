package reader

import (
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	"sync"
)

// Reader is the read face on a pinned Repository commit. It does not write.
// Consumer federation is Serving on a WorkspacePin (coordinates from ResolveWorkspace).
// Catalog does not read object_id.
//
// Tasks, by what they answer (design ch.7):
//
//	identity:  Resolve, Read, ResolveAddress, ReadAddress
//	history:   Log, Diff, GetProvenance
//	schema:    DescribeSchema  (schema/* AccessHints; not a GraphQL runtime)
//	debug:     Search  (JSON contains; production retrieval is Projection)
//
// GroundingCitation is a consume-side projection of a READ result (D12).
type Reader struct {
	store   *snapshot.Registry
	journal journal.Journal
	mu      sync.Mutex
	repos   map[kernel.RepositoryID]knowledge.Repository
}

func NewReader(store *snapshot.Registry) *Reader {
	return &Reader{
		store: store,
		repos: map[kernel.RepositoryID]knowledge.Repository{},
	}
}

func (r *Reader) SetJournal(j journal.Journal) { r.journal = j }

func (r *Reader) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(r.journal, journal.LayerSystem, "reader", cmd, refs, err)
}

func (r *Reader) repoByID(repositoryID kernel.RepositoryID) (knowledge.Repository, error) {
	return r.Require(repositoryID, kernel.ErrKnowledgeRefUnresolved)
}

func (r *Reader) Resolve(ref knowledge.KnowledgeRef, commitID kernel.CommitID) (resolution knowledge.Resolution, err error) {
	defer func() {
		err = r.note("resolve", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	return repo.Resolve(ref.Object, commitID)
}

func (r *Reader) Read(ref knowledge.KnowledgeRef, commitID kernel.CommitID, selector *knowledge.AspectSelector) (value knowledge.KnowledgeValue, err error) {
	defer func() {
		err = r.note("read", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value, err = repo.Read(ref.Object, commitID)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	if selector == nil {
		return value, nil
	}
	value.Value = knowledge.SelectAspects(value.Value, value.Units, selector)
	return value, nil
}

func (r *Reader) ResolveAddress(repositoryID kernel.RepositoryID, address knowledge.Address, commitID kernel.CommitID) (resolution knowledge.Resolution, err error) {
	defer func() {
		err = r.note("resolve-address", map[string]any{"repositoryId": string(repositoryID), "object": string(address.ObjectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	return repo.ResolveAddress(address, commitID)
}

func (r *Reader) ReadAddress(repositoryID kernel.RepositoryID, address knowledge.Address, commitID kernel.CommitID) (value knowledge.KnowledgeValue, err error) {
	defer func() {
		err = r.note("read-address", map[string]any{"repositoryId": string(repositoryID), "object": string(address.ObjectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	return repo.ReadAddress(address, commitID)
}

package reader

import (
	"kc/internal/journal"
	"kc/kernel"
	"kc/repository"
)

// Reader is the read face on a pinned Repository commit. It does not write,
// does not assemble a full ViewReadVersion, and does not own Catalog federation.
//
// Tasks, by what they answer (design ch.7):
//
//	identity:  Resolve, Read, ResolveAddress, ReadAddress, List
//	history:   Log, Diff, GetProvenance
//	schema:    DescribeSchema  (schema/* AccessHints; not a GraphQL runtime)
//	stream:    QueryStream, ReadStream
//	debug:     Search  (JSON contains; production retrieval is Projection)
//
// GroundingCitation is a consume-side projection of a READ result (D12).
// Refine is an optional Ref-preserving capability, not a base read.
type Reader struct {
	store   *repository.Store
	journal journal.Journal
}

func NewReader(store *repository.Store) *Reader {
	return &Reader{store: store}
}

func (r *Reader) SetJournal(j journal.Journal) { r.journal = j }

func (r *Reader) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(r.journal, journal.LayerSystem, "reader", cmd, refs, err)
}

func (r *Reader) repoByID(repositoryID kernel.RepositoryID) (repository.Repository, error) {
	return r.store.Require(repositoryID, kernel.ErrKnowledgeRefUnresolved)
}

func (r *Reader) Resolve(ref kernel.KnowledgeRef, commitID kernel.CommitID) (resolution repository.Resolution, err error) {
	defer func() {
		err = r.note("resolve", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return repository.Resolution{}, err
	}
	return repo.Resolve(ref.Object, commitID)
}

func (r *Reader) Read(ref kernel.KnowledgeRef, commitID kernel.CommitID, selector *repository.AspectSelector) (value repository.KnowledgeValue, err error) {
	defer func() {
		err = r.note("read", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	value, err = repo.Read(ref.Object, commitID)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	if selector == nil {
		return value, nil
	}
	value.Value = repository.SelectAspects(value.Value, value.Units, selector)
	return value, nil
}

func (r *Reader) ResolveAddress(repositoryID kernel.RepositoryID, address kernel.Address, commitID kernel.CommitID) (resolution repository.Resolution, err error) {
	defer func() {
		err = r.note("resolve-address", map[string]any{"repositoryId": string(repositoryID), "object": string(address.ObjectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return repository.Resolution{}, err
	}
	return repo.ResolveAddress(address, commitID)
}

func (r *Reader) ReadAddress(repositoryID kernel.RepositoryID, address kernel.Address, commitID kernel.CommitID) (value repository.KnowledgeValue, err error) {
	defer func() {
		err = r.note("read-address", map[string]any{"repositoryId": string(repositoryID), "object": string(address.ObjectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return repository.KnowledgeValue{}, err
	}
	return repo.ReadAddress(address, commitID)
}

func (r *Reader) List(repositoryID kernel.RepositoryID, commitID kernel.CommitID) (values []repository.KnowledgeValue, err error) {
	defer func() {
		err = r.note("list", map[string]any{"repositoryId": string(repositoryID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return nil, err
	}
	return repo.List(commitID)
}

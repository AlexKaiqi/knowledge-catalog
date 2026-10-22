package reader

import (
	"kc/internal/journal"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"sync"
)

// Reader is the read face on a pinned Repository commit. It does not write.
// Consumer federation is Serving on a DatasetPin (coordinates from ResolveDataset).
// Dataset service does not read object_id.
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
	store    snapshot.Registry
	journal  journal.Journal
	mu       sync.Mutex
	repos    map[kernel.RepositoryID]objectmodel.Repository
	hydrator objectmodel.Hydrator
}

func NewReader(store snapshot.Registry) *Reader {
	return &Reader{
		store: store,
		repos: map[kernel.RepositoryID]objectmodel.Repository{},
	}
}

func (r *Reader) SetJournal(j journal.Journal) { r.journal = j }

func (r *Reader) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(r.journal, journal.LayerSystem, "reader", cmd, refs, err)
}

func (r *Reader) repoByID(repositoryID kernel.RepositoryID) (objectmodel.Repository, error) {
	return r.Require(repositoryID, kernel.ErrObjectRefUnresolved)
}

func (r *Reader) Resolve(ref objectmodel.ObjectRef, commitID kernel.CommitID) (resolution objectmodel.Resolution, err error) {
	defer func() {
		err = r.note("resolve", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	return repo.Resolve(ref.Object, commitID)
}

func (r *Reader) Read(ref objectmodel.ObjectRef, commitID kernel.CommitID) (value objectmodel.ObjectValue, err error) {
	defer func() {
		err = r.note("read", map[string]any{"repositoryId": string(ref.Repository), "object": string(ref.Object), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value, err = readHydrated(r.hydration(), repo, ref.Object, commitID)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value.Digest = kernel.CanonicalDigest(value.Value)
	return value, nil
}

func (r *Reader) ResolveAddress(repositoryID kernel.RepositoryID, address objectmodel.Address, commitID kernel.CommitID) (resolution objectmodel.Resolution, err error) {
	defer func() {
		err = r.note("resolve-address", map[string]any{"repositoryId": string(repositoryID), "object": string(address.ObjectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	return repo.ResolveAddress(address, commitID)
}

func (r *Reader) ReadAddress(repositoryID kernel.RepositoryID, address objectmodel.Address, commitID kernel.CommitID) (value objectmodel.ObjectValue, err error) {
	defer func() {
		err = r.note("read-address", map[string]any{"repositoryId": string(repositoryID), "object": string(address.ObjectID), "commit": string(commitID)}, err)
	}()
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	value, err = readAddressHydrated(r.hydration(), repo, address, commitID)
	if err == nil {
		value.Digest = kernel.CanonicalDigest(value.Value)
	}
	return value, err
}

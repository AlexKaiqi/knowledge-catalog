package repository

import "kc/kernel"

// SnapshotStore is layer ⓪: git-shaped authority (tree / commit / ref / CAS).
// A Catalog registers SnapshotStores. Hang by git URL + read grant; the
// registry does not take object bytes. Pin checks HasCommit, not Aspect.
// The COMMIT from→to event is the Snapshot struct in snapshot.go, not this.
type SnapshotStore interface {
	ID() kernel.RepositoryID

	Head(ref string) (kernel.CommitID, error)
	GetRef(ref string) (kernel.CommitID, bool)
	HasCommit(commitID kernel.CommitID) bool
	CreateRef(ref string, commitID kernel.CommitID) error
	Merge(targetRef string, candidate, expected kernel.CommitID) (kernel.CommitID, error)
	ApplyCommit(cs CommitChangeSet) (kernel.CommitID, error)

	Archived() bool
	Archive() error
}

// Stream is layer ⓪: ordered log (cursor / eventId). Not git, not a Catalog
// snapshot member. Do not repo-add a stream.
type Stream interface {
	Append(streamRef string, entries []AppendEntry, expectedCursor string) ([]string, error)
	StreamCursor(streamRef string) string
	ReadStream(streamRef string) StreamSlice
}

// Knowledge is layer ②: interpret Snapshot files at a commit (object_id,
// Aspect, provenance). Catalog pin does not go through here.
type Knowledge interface {
	Resolve(objectID kernel.ObjectID, commitID kernel.CommitID) (Resolution, error)
	Read(objectID kernel.ObjectID, commitID kernel.CommitID) (KnowledgeValue, error)
	ResolveAddress(address kernel.Address, commitID kernel.CommitID) (Resolution, error)
	ReadAddress(address kernel.Address, commitID kernel.CommitID) (KnowledgeValue, error)
	GetProvenance(objectID kernel.ObjectID, commitID kernel.CommitID) (ProvenanceTrace, error)
	Log(objectID kernel.ObjectID, commitID kernel.CommitID, limit int) ([]ObjectRevision, error)
	Diff(objectID kernel.ObjectID, from, to kernel.CommitID) (ObjectDiff, error)
	Search(query string, commitID kernel.CommitID) ([]KnowledgeValue, error)
	List(commitID kernel.CommitID) ([]KnowledgeValue, error)
}

// Repository is a Snapshot member that can also interpret knowledge files.
// APPEND is not here: bind a Stream on Store (JSONL beside FileGit/Dolt is
// packing, not a Catalog member). See docs/LAYERS.md.
type Repository interface {
	SnapshotStore
	Knowledge
}

// Surface is which write path a change takes.
// COMMIT/PROPOSAL target a SnapshotStore; APPEND targets a Stream.
// PUT/REMOVE on a ChangeSet are layer ② (Aspect partitions).
type Surface string

const (
	SurfaceCommit   Surface = "COMMIT"
	SurfaceProposal Surface = "PROPOSAL"
	SurfaceAppend   Surface = "APPEND"
)

// Store is opened member Repositories and their bound Streams, keyed by id.
// The stream key is the Snapshot id for ACL/collocation; the stream is not
// the Catalog member. Not a Catalog object.
type Store struct {
	repos      map[kernel.RepositoryID]Repository
	streams    map[kernel.RepositoryID]Stream
	onSnapshot []func(Snapshot)
}

func NewStore() *Store {
	return &Store{
		repos:   map[kernel.RepositoryID]Repository{},
		streams: map[kernel.RepositoryID]Stream{},
	}
}

func (s *Store) Add(repo Repository) error {
	if _, ok := s.repos[repo.ID()]; ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "repository %s is already registered", repo.ID())
	}
	s.repos[repo.ID()] = repo
	return nil
}

func (s *Store) AddStream(id kernel.RepositoryID, stream Stream) error {
	if _, ok := s.streams[id]; ok {
		return kernel.Fail(kernel.ErrPreconditionFailed, "stream %s is already registered", id)
	}
	s.streams[id] = stream
	return nil
}

func (s *Store) Get(id kernel.RepositoryID) (Repository, bool) {
	r, ok := s.repos[id]
	return r, ok
}

func (s *Store) GetStream(id kernel.RepositoryID) (Stream, bool) {
	stream, ok := s.streams[id]
	return stream, ok
}

func (s *Store) Delete(id kernel.RepositoryID) {
	delete(s.repos, id)
	delete(s.streams, id)
}

func (s *Store) Require(id kernel.RepositoryID, code kernel.ErrorCode) (Repository, error) {
	r, ok := s.repos[id]
	if !ok {
		switch code {
		case kernel.ErrTemporaryUnavailable:
			return nil, kernel.Fail(code, "repository %s is not mounted", id)
		default:
			return nil, kernel.Fail(code, "unknown repository %s", id)
		}
	}
	return r, nil
}

func (s *Store) RequireStream(id kernel.RepositoryID, code kernel.ErrorCode) (Stream, error) {
	stream, ok := s.streams[id]
	if !ok {
		switch code {
		case kernel.ErrTemporaryUnavailable:
			return nil, kernel.Fail(code, "stream %s is not mounted", id)
		default:
			return nil, kernel.Fail(code, "unknown stream %s", id)
		}
	}
	return stream, nil
}

func (s *Store) IDs() []kernel.RepositoryID {
	ids := make([]kernel.RepositoryID, 0, len(s.repos))
	for id := range s.repos {
		ids = append(ids, id)
	}
	return ids
}

// Close releases adapters that hold process resources (Postgres pools).
func (s *Store) Close() error {
	var first error
	closeOne := func(v any) {
		if c, ok := v.(interface{ Close() error }); ok {
			if err := c.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	for id, repo := range s.repos {
		closeOne(repo)
		delete(s.repos, id)
	}
	for id, stream := range s.streams {
		closeOne(stream)
		delete(s.streams, id)
	}
	return first
}

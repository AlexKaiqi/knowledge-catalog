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
// snapshot member. Do not repo-add a stream. Catalog may freeze StreamRefs()
// plus StreamCursor as AppendCuts; it does not read payloads.
type Stream interface {
	Append(streamRef string, entries []AppendEntry, expectedCursor string) ([]string, error)
	StreamCursor(streamRef string) string
	StreamRefs() []string
	ReadStream(streamRef string) StreamSlice
}

// StreamAvailability is implemented by a mounted placeholder that exists so a
// Snapshot can still open while its deployment's Stream engine is unavailable.
// Stream's legacy read methods cannot return errors, so readers must ask this
// optional capability before treating an empty slice as a real empty stream.
// A production Stream does not implement this interface.
type StreamAvailability interface {
	StreamAvailabilityError() error
}

func CheckStreamAvailable(stream Stream) error {
	if availability, ok := stream.(StreamAvailability); ok {
		return availability.StreamAvailabilityError()
	}
	return nil
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
// Knowledge is an optional capability, not an entry ticket: a plain git repo
// mounts as a SnapshotStore and still composes, checks out and takes writes by
// path. Ask Store.Knowledge only where layer ② is actually needed.
// APPEND is not here: bind a Stream on Store (JSONL beside FileGit/Dolt is
// packing, not a Catalog member). See docs/LAYERS.md, docs/COMPOSITION.md.
type Repository interface {
	SnapshotStore
	Knowledge
}

// RawFileStore is an optional layer ⓪ capability, sibling to Knowledge: read
// and write bytes at a literal path, with no frontmatter interpretation and
// no object_id. It is the shape a plain path-addressed filesystem consumer
// actually calls with — e.g. a virtual workspace that hands an external agent
// harness the composed tree without ever materializing it on disk (a mount's
// writable git worktree, per docs/COMPOSITION.md §2.3, still exists for that
// case; RawFileStore is for callers that never get a worktree at all).
// A member can have RawFileStore, Knowledge, both, or neither; Store.Add
// still only requires SnapshotStore.
type RawFileStore interface {
	ReadFile(path string, commit kernel.CommitID) ([]byte, error)
	ListFiles(commit kernel.CommitID) ([]string, error)
	ApplyRawCommit(cs RawFileChangeSet) (kernel.CommitID, error)
}

// RawFileStoreOf is the capability check for RawFileStore, mirroring KnowledgeOf.
func RawFileStoreOf(snapshot SnapshotStore) (RawFileStore, bool) {
	r, ok := snapshot.(RawFileStore)
	return r, ok
}

// RawFileChange is one literal-path write or delete: no Address, no
// object_id, no schema_ref. Content is ignored when Remove is true.
type RawFileChange struct {
	Path    string `json:"path"`
	Content []byte `json:"content,omitempty"`
	Remove  bool   `json:"remove,omitempty"`
}

// RawFileChangeSet is CommitChangeSet's sibling for RawFileStore: the same
// CAS shape (BaseCommit/ExpectedTargetCommit must be equal, same as
// CommitChangeSet), no Operations/Address at all.
type RawFileChangeSet struct {
	TargetRepository     kernel.RepositoryID `json:"targetRepository"`
	TargetRef            string              `json:"targetRef"`
	BaseCommit           kernel.CommitID     `json:"baseCommit"`
	ExpectedTargetCommit kernel.CommitID     `json:"expectedTargetCommit"`
	Changes              []RawFileChange     `json:"changes"`
	Message              string              `json:"message,omitempty"`
	Author               string              `json:"author,omitempty"`
	RequestID            string              `json:"requestId,omitempty"`
	RuleID               string              `json:"ruleId,omitempty"`
}

// DefaultRef is the branch a COMMIT or PROPOSAL targets when the caller names
// no ref. It is a protocol default, not a git mechanism: a Snapshot adapter that
// is not git still has to answer for this ref.
const DefaultRef = "refs/heads/main"

// RefOrDefault fills in DefaultRef for an omitted ref.
func RefOrDefault(ref string) string {
	if ref == "" {
		return DefaultRef
	}
	return ref
}

// Surface is which write path a change takes.
// COMMIT/PROPOSAL target a SnapshotStore; APPEND targets a Stream.
// PUT/REMOVE on a ChangeSet are layer ② (Aspect partitions).
type Surface string

const (
	SurfaceCommit   Surface = "COMMIT"
	SurfaceProposal Surface = "PROPOSAL"
	SurfaceAppend   Surface = "APPEND"
	// SurfaceRawWrite targets RawFileStore: literal path bytes, no Address.
	SurfaceRawWrite Surface = "RAW_WRITE"
)

// Store is opened member SnapshotStores and their bound Streams, keyed by id.
// Membership requires layer ⓪ only; interpreting knowledge files is a capability
// some members happen to have. The stream key is the Snapshot id for
// ACL/collocation; the stream is not the Catalog member. Not a Catalog object.
type Store struct {
	repos      map[kernel.RepositoryID]SnapshotStore
	streams    map[kernel.RepositoryID]Stream
	onSnapshot []func(Snapshot)
}

func NewStore() *Store {
	return &Store{
		repos:   map[kernel.RepositoryID]SnapshotStore{},
		streams: map[kernel.RepositoryID]Stream{},
	}
}

func (s *Store) Add(repo SnapshotStore) error {
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

func (s *Store) Get(id kernel.RepositoryID) (SnapshotStore, bool) {
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

func (s *Store) Require(id kernel.RepositoryID, code kernel.ErrorCode) (SnapshotStore, error) {
	r, ok := s.repos[id]
	if !ok {
		switch code {
		case kernel.ErrUsageInvalid:
			return nil, kernel.Fail(code, "repository %s is not mounted", id)
		default:
			return nil, kernel.Fail(code, "unknown repository %s", id)
		}
	}
	return r, nil
}

// Knowledge requires the member to also interpret knowledge files (layer ②).
// Mounted-but-plain is not an error of mounting: it is a missing capability,
// so it reports CAPABILITY_UNSATISFIED rather than the caller's not-found code.
func (s *Store) Knowledge(id kernel.RepositoryID, code kernel.ErrorCode) (Repository, error) {
	snapshot, err := s.Require(id, code)
	if err != nil {
		return nil, err
	}
	repo, ok := snapshot.(Repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s is mounted as a plain snapshot and does not interpret knowledge files", id)
	}
	return repo, nil
}

// KnowledgeOf is the capability check without the not-found path.
func KnowledgeOf(snapshot SnapshotStore) (Repository, bool) {
	repo, ok := snapshot.(Repository)
	return repo, ok
}

func (s *Store) RequireStream(id kernel.RepositoryID, code kernel.ErrorCode) (Stream, error) {
	stream, ok := s.streams[id]
	if !ok {
		switch code {
		case kernel.ErrUsageInvalid:
			return nil, kernel.Fail(code, "stream for repository %s is not mounted", id)
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

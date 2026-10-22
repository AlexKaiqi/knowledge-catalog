package projection

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"os"
	"regexp"
	"sync"

	"kc/kernel"
	objectmodel "kc/object"
)

// Index sits above one Repository (K-19): derived, discardable, never Canonical.
// Independent of Writer / Reader / Dataset service. Subscribe via dataset.Hook (Sink).
// Live key is repository id (commit empty). Pin key is (repository, basisCommit).
type Index struct {
	dir  string
	open ports.EngineOpener
	mu   sync.Mutex
	// stateBuildMu serializes refresh publishers without blocking searches while
	// a new warm generation is compiled.
	stateBuildMu sync.Mutex
	// fixedBuildGate keeps builds of the same projection basis mutually
	// exclusive. It replaces a plain process-wide mutex because waiting for a
	// build must observe the caller's own cancellation: a client that already
	// timed out must not stay queued behind a large repository scan. Deciding
	// that a projection is READY is read-only and never takes the gate.
	gateMu                   sync.Mutex
	fixedBuildGate           map[engineKey]chan struct{}
	stateMu                  sync.RWMutex
	engs                     map[engineKey]ports.Engine
	states                   map[engineKey]*stateProjection
	stateDirectory           string
	temporaryStateDirectory  bool
	hydrator                 objectmodel.Hydrator
	check                    retrieval.DeclarationCheck
	manifestDigest           kernel.Digest
	datasetRevisionFence     DatasetRevisionFence
	resourceCoordinator      ports.ResourceCoordinator
	publicationRecovery      func(context.Context, []byte) (bool, error)
	publicationReference     func(context.Context, []byte) (bool, error)
	publicationEvidence      func(context.Context, string, int) ([]byte, error)
	publishedDatasetRecovery func(context.Context, string, int) error
	publishedRecoveryGate    chan struct{}
	observationPrepare       func(context.Context, string, int) (func(func() (ports.PreparedResource, error)) error, error)
	observationEvidence      func(context.Context, string, int) ([]ports.ObservationReference, error)
}

// DatasetRevisionFence orders in-process observation refresh:
// it runs accept while revision stays the current, unretired revision of the
// Dataset, and fails with PRECONDITION_FAILED otherwise. Publication of that
// Dataset waits for accept in this process. Actual cross-process acceptance
// additionally requires the injected observation publication authority.
type DatasetRevisionFence func(setID string, revision int, accept func() error) error

// SetDatasetRevisionFence supplies the publication authority for observation
// refresh. Retained revisions remain queryable, but only current is refreshed.
func (idx *Index) SetDatasetRevisionFence(fence DatasetRevisionFence) {
	idx.datasetRevisionFence = fence
}

// NewIndexEngine builds one working projection using the supplied physical engine.
func NewIndexEngine(dir string, opener ports.EngineOpener) *Index {
	if opener == nil {
		opener = func(string, kernel.RepositoryID) (ports.Engine, error) {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index engine opener required (retrieval/opensearch)")
		}
	}
	return &Index{dir: dir, stateDirectory: dir, open: opener, engs: map[engineKey]ports.Engine{}, states: map[engineKey]*stateProjection{}, fixedBuildGate: map[engineKey]chan struct{}{}}
}

// enterFixedBuild acquires the build gate for one projection basis. Waiting
// honours ctx so a caller whose deadline expired is released instead of
// blocking on a scan it did not ask for. The returned function releases the
// gate exactly once.
//
// The gate is per projection, not per process: builds of different bases share
// no state, so making them queue behind each other only spreads one slow
// scan's latency across unrelated work. Entries are retained for the process
// lifetime so a waiter and a newcomer always agree on the same channel.
func (idx *Index) enterFixedBuild(ctx context.Context, key engineKey) (func(), error) {
	idx.gateMu.Lock()
	gate := idx.fixedBuildGate[key]
	if gate == nil {
		gate = make(chan struct{}, 1)
		idx.fixedBuildGate[key] = gate
	}
	idx.gateMu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// AfterSnapshot applies a member snapshot to the working projection.
// Dataset service never imports this package; CLI / tests wrap dataset.Hook.
func (idx *Index) AfterSnapshot(repo objectmodel.Repository, from, to kernel.CommitID, objectIDs []objectmodel.ObjectID) error {
	if idx == nil || repo == nil {
		return nil
	}
	if objectIDs == nil {
		_, err := idx.Ensure(context.Background(), repo, to)
		return err
	}
	_, err := idx.Apply(repo, from, to, objectIDs)
	return err
}

func (idx *Index) Close() error {
	idx.stateBuildMu.Lock()
	defer idx.stateBuildMu.Unlock()
	idx.stateMu.Lock()
	defer idx.stateMu.Unlock()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	var first error
	for key, eng := range idx.engs {
		if err := eng.Close(); err != nil && first == nil {
			first = err
		}
		delete(idx.engs, key)
	}
	for key, state := range idx.states {
		if err := state.store.release(); err != nil && first == nil {
			first = err
		}
		delete(idx.states, key)
	}
	idx.states = map[engineKey]*stateProjection{}
	if idx.temporaryStateDirectory {
		if err := os.RemoveAll(idx.stateDirectory); err != nil && first == nil {
			first = err
		}
		idx.stateDirectory = ""
		idx.temporaryStateDirectory = false
	}
	return first
}

var unsafeIndexChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func SanitizeID(id string) string {
	s := unsafeIndexChars.ReplaceAllString(id, "_")
	if s == "" {
		return "repo"
	}
	return s
}

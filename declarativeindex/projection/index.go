package projection

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
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
	stateBuildMu         sync.Mutex
	fixedBuildMu         sync.Mutex
	stateMu              sync.RWMutex
	engs                 map[engineKey]ports.Engine
	states               map[engineKey]*stateProjection
	hydrator             objectmodel.Hydrator
	check                retrieval.DeclarationCheck
	manifestDigest       kernel.Digest
	datasetRevisionFence DatasetRevisionFence
}

// DatasetRevisionFence is the publication authority for observation refresh:
// it runs accept while revision stays the current, unretired revision of the
// Dataset, and fails with PRECONDITION_FAILED otherwise. Publication of that
// Dataset waits for accept, so a refresh is never accepted for a revision
// that was replaced while it committed.
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
	return &Index{dir: dir, open: opener, engs: map[engineKey]ports.Engine{}, states: map[engineKey]*stateProjection{}}
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
		if err := state.store.CloseAndRemove(); err != nil && first == nil {
			first = err
		}
		delete(idx.states, key)
	}
	idx.states = map[engineKey]*stateProjection{}
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

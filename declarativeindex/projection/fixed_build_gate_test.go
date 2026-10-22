package projection_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"kc/declarativeindex/internal/testfixture"
	index "kc/declarativeindex/projection"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// countingReferrerRepository records how often the Schema referrer locator is
// enumerated. That enumeration is the O(references) authority walk behind a
// full contract compile, so its count is the direct measure of whether a
// readiness question was answered from metadata or from the whole repository.
type countingReferrerRepository struct {
	*testkit.ObjectRepository
	mu       sync.Mutex
	referrer int
	reads    int
}

func (r *countingReferrerRepository) SchemaReferrerAddresses(id objectmodel.ObjectID, at kernel.CommitID) ([]objectmodel.Address, error) {
	r.mu.Lock()
	r.referrer++
	r.mu.Unlock()
	return r.ObjectRepository.SchemaReferrerAddresses(id, at)
}

func (r *countingReferrerRepository) ReadAddress(address objectmodel.Address, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	r.mu.Lock()
	r.reads++
	r.mu.Unlock()
	return r.ObjectRepository.ReadAddress(address, at)
}

func (r *countingReferrerRepository) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.referrer, r.reads
}

func (r *countingReferrerRepository) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.referrer, r.reads = 0, 0
}

func seededRepository(t *testing.T) (*testkit.ObjectRepository, kernel.CommitID) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://projection/fixed-gate")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"},
				Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
			testkit.IndexText("schema/article", "body"),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/a"},
				Value: map[string]any{"body": "alpha"}, SchemaRef: "schema/article"},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/b"},
				Value: map[string]any{"body": "beta"}, SchemaRef: "schema/article"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, commit
}

// TestEnsureAtOnReadyBasisDoesNotWalkTheReferrerIndex locks in the property that
// makes a large published repository usable: once a basis is READY, asking for
// it again must not enumerate references. A regression here is not a slow
// query, it is every concurrent definition waiting on a full repository scan.
func TestEnsureAtOnReadyBasisDoesNotWalkTheReferrerIndex(t *testing.T) {
	repo, commit := seededRepository(t)
	counted := &countingReferrerRepository{ObjectRepository: repo}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) {
		return &testfixture.StateEngine{}, nil
	})
	t.Cleanup(func() { _ = idx.Close() })

	built, err := idx.EnsureAt(context.Background(), counted, commit)
	if err != nil {
		t.Fatal(err)
	}
	if built.BasisCommit != commit {
		t.Fatalf("first EnsureAt did not build the basis: %+v", built)
	}

	counted.reset()
	again, err := idx.EnsureAt(context.Background(), counted, commit)
	if err != nil {
		t.Fatal(err)
	}
	if again.Mode != ports.IndexModeReady || again.BasisCommit != commit {
		t.Fatalf("second EnsureAt must answer READY: %+v", again)
	}
	if referrer, reads := counted.counts(); referrer != 0 || reads != 0 {
		t.Fatalf("READY basis re-asked the referrer index: enumerations=%d address reads=%d", referrer, reads)
	}
}

// blockingEngine holds the first rebuild until released, which is how the test
// keeps one build inside the gate while a second caller tries to enter it.
// StateEngine rebuilds through a streaming session, so the hold sits on the
// session commit rather than on Rebuild.
type blockingEngine struct {
	*testfixture.StateEngine
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockingEngine) BeginRebuild(meta ports.Meta) (ports.RebuildSession, error) {
	session, err := e.StateEngine.BeginRebuild(meta)
	if err != nil {
		return nil, err
	}
	return &blockingSession{RebuildSession: session, engine: e}, nil
}

type blockingSession struct {
	ports.RebuildSession
	engine *blockingEngine
}

func (s *blockingSession) Commit() error {
	s.engine.once.Do(func() { close(s.engine.entered) })
	<-s.engine.release
	return s.RebuildSession.Commit()
}

func TestEnsureAtWaitingOnBuildHonoursCancellation(t *testing.T) {
	repo, commit := seededRepository(t)
	engine := &blockingEngine{StateEngine: &testfixture.StateEngine{}, entered: make(chan struct{}), release: make(chan struct{})}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })

	holder := make(chan error, 1)
	go func() {
		_, err := idx.EnsureAt(context.Background(), repo, commit)
		holder <- err
	}()
	select {
	case <-engine.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("first build never entered the gate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := idx.EnsureAt(ctx, repo, commit)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting caller returned %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("cancellation took %s, gate wait ignores ctx", elapsed)
	}

	close(engine.release)
	if err := <-holder; err != nil {
		t.Fatalf("first build failed: %v", err)
	}
}

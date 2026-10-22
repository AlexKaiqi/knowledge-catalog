package projection

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
)

type unavailableObservationEngine struct {
	testfixture.StateEngine
	failure   error
	positions []string
}

func (e *unavailableObservationEngine) BindObservation(_ context.Context, position string) (ports.Engine, error) {
	e.positions = append(e.positions, position)
	return nil, e.failure
}

func TestObservationSearchDoesNotFallbackAfterBindingFailure(t *testing.T) {
	repo, at, _ := testfixture.StateProjection(t)
	problem := kernel.Fail(kernel.ErrTemporaryUnavailable, "observation lease authority unavailable")
	engine := &unavailableObservationEngine{failure: problem}
	idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	defer idx.Close()
	lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-10-21T00:00:00Z"}}
	if _, err := idx.RefreshState(context.Background(), repo, at, sourcebridge.Source{Lookup: lookup}); err != nil {
		t.Fatal(err)
	}
	state := idx.states[stateStoreKey(repo.ID(), at)]
	// The current cached values are valid. A provider failure still cannot be
	// converted into a current-generation search or a successful empty page.
	for _, position := range []string{"", "original-provider-position"} {
		request := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "running"})
		if position != "" {
			request.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{Scope: "repository", Position: position})
		}
		result, err := idx.SearchStateAtRevisionContext(context.Background(), repo, at, "", request)
		if !errors.Is(err, problem) || len(result.Hits) != 0 || result.Completeness != "" {
			t.Fatalf("binding error became successful query: %#v %v", result, err)
		}
	}
	if len(engine.positions) != 2 || engine.positions[1] != "original-provider-position" || lookup.Calls != 1 || idx.states[stateStoreKey(repo.ID(), at)] != state {
		t.Fatalf("failure changed cache, called runtime or discarded cursor: %v calls=%d", engine.positions, lookup.Calls)
	}
}

func TestObservationCandidatesUseIndependentFilesForConcurrentBuilders(t *testing.T) {
	directory := t.TempDir()
	key := stateStoreKey("kr://test/shared", "fixed")
	const count = 16
	var wg, opened sync.WaitGroup
	opened.Add(count)
	release := make(chan struct{})
	names := make(chan string, count)
	failures := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store, err := openStateServingStore(directory, key, "building")
			if err != nil {
				failures <- err
				opened.Done()
				return
			}
			names <- store.file
			opened.Done()
			<-release
			if err := store.CloseAndRemove(); err != nil {
				failures <- err
			}
		}()
	}
	opened.Wait()
	close(release)
	wg.Wait()
	close(names)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for name := range names {
		if seen[name] {
			t.Fatalf("builders share mutable inode path: %s", name)
		}
		seen[name] = true
	}
	if len(seen) != count {
		t.Fatalf("lost candidates: %d", len(seen))
	}
}

func TestObservationDigestCancellationIncludesEmptyStores(t *testing.T) {
	store, err := openStateServingStore(t.TempDir(), stateStoreKey("scope", "fixed"), "building")
	if err != nil {
		t.Fatal(err)
	}
	defer store.CloseAndRemove()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.DigestContext(ctx, "fixed"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled empty digest reported success: %v", err)
	}
	if _, _, err := readObservationFile(ctx, store.file); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled file read reached medium: %v", err)
	}
	if _, err := os.Stat(store.file); err != nil {
		t.Fatalf("canceled read deleted source: %v", err)
	}
}

func TestObservationCandidateCleanupDoesNotPruneSharedDirectoryOrReplaceExistingFile(t *testing.T) {
	directory := t.TempDir()
	key := stateStoreKey("shared", "fixed")
	store, err := openStateServingStore(directory, key, "named")
	if err != nil {
		t.Fatal(err)
	}
	file, shared := store.file, store.dir
	if _, err := openStateServingStore(directory, key, "named"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("candidate reopened another builder's file: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("failed contender removed owned file: %v", err)
	}
	if err := store.CloseAndRemove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("candidate cleanup removed shared namespace: %v", err)
	}
}

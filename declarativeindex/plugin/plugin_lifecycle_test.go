package plugin_test

import (
	"context"
	"errors"
	consumerplugin "kc/discovery/plugin"
	"runtime"
	"sync"
	"testing"
	"time"

	retrieval "kc/discovery"

	indexcontrol "kc/declarativeindex/control"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestIndexPluginsCloseRejectsNewTasksAndWaitsForActualRetrieval(t *testing.T) {
	f := newPluginFixture(t)
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := holdPluginEngine(t, f.pool, false)
	t.Cleanup(func() { release(); _ = plugins.Close() })
	source := f.setups[0]
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.PrepareServingBasis(context.Background(), repo, at); err != nil {
		t.Fatal(err)
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
	type response struct {
		result retrieval.SearchResult
		err    error
	}
	finished := make(chan response, 1)
	go func() {
		result, err := plugins.SearchAtContext(context.Background(), repo, at, query)
		finished <- response{result, err}
	}()
	awaitPluginSignal(t, entered, "actual physical retrieval")
	closed := make(chan error, 1)
	go func() { closed <- plugins.Close() }()
	awaitPluginStopped(t, plugins, repo, at)
	before := f.pool.operations.Load()
	if _, err := plugins.SearchAtContext(context.Background(), repo, at, query); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || f.pool.operations.Load() != before {
		t.Fatalf("new task entered backend during shutdown: %v", err)
	}
	assertPluginStillWaiting(t, closed, f.pool)
	release()
	select {
	case got := <-finished:
		assertPluginHit(t, got.result, got.err, source.RepositoryID, at, "runbook source 1 at a")
	case <-time.After(5 * time.Second):
		t.Fatal("inflight actual retrieval did not finish")
	}
	assertPluginClosed(t, plugins, closed, f.pool)
}

func TestIndexPluginsCloseWaitsForActualCatchUpAndRejectsFurtherCatchUp(t *testing.T) {
	f := newPluginFixture(t)
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := holdPluginEngine(t, f.pool, true)
	t.Cleanup(func() { release(); _ = plugins.Close() })
	source := f.setups[0]
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	workDone := make(chan struct{})
	t.Cleanup(func() {
		release()
		awaitPluginSignal(t, workDone, "accepted catch-up cleanup")
	})
	go func() {
		defer close(workDone)
		finished <- plugins.CatchUp(context.Background())
	}()
	awaitPluginSignal(t, entered, "actual catch-up physical publication")
	closed := make(chan error, 1)
	go func() { closed <- plugins.Close() }()
	awaitPluginStopped(t, plugins, repo, at)
	assertPluginStillWaiting(t, closed, f.pool)
	release()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("accepted actual catch-up failed on graceful close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("accepted actual catch-up did not finish")
	}
	assertPluginClosed(t, plugins, closed, f.pool)
	before := f.pool.operations.Load()
	if err := plugins.CatchUp(context.Background()); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || f.pool.operations.Load() != before {
		t.Fatalf("stopped runtime accepted maintenance: %v", err)
	}
}

func TestIndexPluginsPartialOpenAndStartFailureCloseActualResourcesInReverse(t *testing.T) {
	for _, phase := range []string{"open", "start"} {
		t.Run(phase, func(t *testing.T) {
			f := newPluginFixture(t)
			fault := errors.New("second acquired instance failed " + phase)
			entry := f.entry(t)
			open := entry.Open
			entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
				contribution, err := open(scope)
				if err != nil {
					return contribution, err
				}
				contribution.Maintenance.(*indexcontrol.Controller).SetReconcileInterval(time.Hour)
				for _, id := range scope.Repositories {
					repo, err := f.lookup(id)
					if err != nil {
						return contribution, err
					}
					at, err := repo.Head(snapshot.DefaultRef)
					if err != nil {
						return contribution, err
					}
					if _, err := contribution.Queries.Ensure(repo, at); err != nil {
						return contribution, err
					}
				}
				if scope.Instance == "main-b" {
					if phase == "open" {
						return contribution, fault
					}
					start := contribution.Start
					contribution.Start = func(ctx context.Context) error {
						if err := start(ctx); err != nil {
							return err
						}
						return fault
					}
				}
				return contribution, nil
			}
			plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
			if phase == "open" {
				if !errors.Is(err, fault) || plugins != nil {
					t.Fatalf("partial actual open was accepted: runtime=%v error=%v", plugins, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = plugins.Close() })
				if err := plugins.Start(context.Background()); !errors.Is(err, fault) {
					t.Fatalf("partial actual worker activation was accepted: %v", err)
				}
				source := f.setups[0]
				repo, err := f.lookup(source.RepositoryID)
				if err != nil {
					t.Fatal(err)
				}
				at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
				if _, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
					t.Fatalf("failed activation left usable runtime: %v", err)
				}
				if err := plugins.Close(); err != nil {
					t.Fatal(err)
				}
			}
			f.pool.mu.Lock()
			closes := append([]string(nil), f.pool.closes...)
			f.pool.mu.Unlock()
			assertPluginReverseInstances(t, closes)
			assertPluginEngineCloseCounts(t, f.pool, 1)
		})
	}
}

func assertPluginReverseInstances(t *testing.T, closes []string) {
	t.Helper()
	seenA, seenB := false, false
	for _, namespace := range closes {
		switch namespace {
		case "declarative-index/main-a":
			seenA = true
		case "declarative-index/main-b":
			if seenA {
				t.Fatalf("actual acquired physical resources did not close in reverse: %v", closes)
			}
			seenB = true
		default:
			t.Fatalf("unknown instance resource: %v", closes)
		}
	}
	if !seenA || !seenB {
		t.Fatalf("partial open did not acquire/close both real instances: %v", closes)
	}
}

func holdPluginEngine(t *testing.T, pool *pluginEnginePool, rebuild bool) (<-chan struct{}, func()) {
	t.Helper()
	pool.entered, pool.release = make(chan struct{}), make(chan struct{})
	if rebuild {
		pool.blockRebuild.Store(true)
	} else {
		pool.block.Store(true)
	}
	var once sync.Once
	return pool.entered, func() { once.Do(func() { close(pool.release) }) }
}

func awaitPluginSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never entered physical backend", operation)
	}
}

func awaitPluginStopped(t *testing.T, plugins *consumerplugin.IndexPlugins, repo objectmodel.Repository, at kernel.CommitID) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := plugins.AccessSpecAt(repo, at); kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return
		} else if err != nil {
			t.Fatalf("shutdown observation: %v", err)
		}
		runtime.Gosched()
	}
	t.Fatal("close did not stop admitting new tasks")
}

func assertPluginStillWaiting(t *testing.T, closed <-chan error, pool *pluginEnginePool) {
	t.Helper()
	select {
	case err := <-closed:
		t.Fatalf("runtime closed while actual physical work remained in flight: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	pool.mu.Lock()
	var held []*pluginPhysicalEngine
	for handle := range pool.held {
		held = append(held, handle)
	}
	pool.mu.Unlock()
	if len(held) == 0 {
		t.Fatal("lifecycle assertion did not hold actual physical work")
	}
	for _, handle := range held {
		if got := handle.closed.Load(); got != 0 {
			t.Fatalf("inflight physical resource %s/%s closed %d times", handle.namespace, handle.repository, got)
		}
	}
}

func assertPluginClosed(t *testing.T, plugins *consumerplugin.IndexPlugins, closed <-chan error, pool *pluginEnginePool) {
	t.Helper()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not close after accepted work finished")
	}
	assertPluginEngineCloseCounts(t, pool, 1)
	if err := plugins.Close(); err != nil {
		t.Fatal(err)
	}
	assertPluginEngineCloseCounts(t, pool, 1)
}

func assertPluginEngineCloseCounts(t *testing.T, pool *pluginEnginePool, want int64) {
	t.Helper()
	pool.mu.Lock()
	handles := append([]*pluginPhysicalEngine(nil), pool.handles...)
	pool.mu.Unlock()
	if len(handles) == 0 {
		t.Fatal("lifecycle assertion did not acquire an actual engine")
	}
	for _, handle := range handles {
		if got := handle.closed.Load(); got != want {
			t.Fatalf("physical resource %s/%s close count=%d, want %d", handle.namespace, handle.repository, got, want)
		}
	}
}

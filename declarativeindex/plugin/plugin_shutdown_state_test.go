package plugin_test

import (
	"context"
	consumerplugin "kc/discovery/plugin"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	declarative "kc/declarativeindex/plugin"
	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"

	// Goal: declarative-index U6 and plugin-extension's lifecycle boundary must
	// stop background work before draining a task waiting for its Controller lock.
	// The selected path uses the actual Entry, Index, Controller and Writer-backed
	// Bound declaration. Only the physical engine and the State source are ports;
	// Non-Goals: no task implementation, source commit or cancellation policy is
	// replaced, and no concrete index migration or shutdown timeout is introduced.
	indexcontrol "kc/declarativeindex/control"
)

func TestIndexPluginsCloseCancelsBackgroundStateBeforeDrainingCatchUp(t *testing.T) {
	f := newPluginFixture(t)
	source := f.setups[0]
	base := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	commit, err := source.Writer.Commit("bound-shutdown-state", objectmodel.CommitChangeSet{
		TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "job/shutdown"}, Value: map[string]any{"name": "shutdown"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime", "origin": "https://scheduler.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	entry := shutdownStateEntry(t, f)
	open := entry.Open
	var queries *indeximplementation.Index
	var foreground *shutdownCatchUp
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err != nil {
			return contribution, err
		}
		controller := contribution.Maintenance.(*indexcontrol.Controller)
		controller.SetReconcileInterval(time.Hour)
		queries = contribution.Queries.(*indeximplementation.Index)
		foreground = &shutdownCatchUp{Controller: controller, entered: make(chan struct{}), drained: make(chan struct{}), release: make(chan struct{})}
		contribution.Maintenance = foreground
		return contribution, nil
	}
	config := consumerplugin.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]consumerplugin.RepositoryPluginConfig{
		source.RepositoryID: {Version: 1, Plugins: []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "shutdown"}}},
	}}
	plugins, err := consumerplugin.OpenIndexPlugins(config, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	lookup := &shutdownStateLookup{entered: make(chan liveaccess.StateLookupRequest, 1), canceled: make(chan struct{}), release: make(chan struct{})}
	startContext, cancelStart := context.WithCancel(context.Background())
	requestContext, cancelRequest := context.WithCancel(context.Background())
	closed := make(chan struct{})
	var closeErr error
	var closeOnce sync.Once
	closeRuntime := func() {
		closeOnce.Do(func() {
			go func() { closeErr = plugins.Close(); close(closed) }()
		})
	}
	t.Cleanup(func() {
		// Independent releases keep a broken shutdown order from hanging the
		// test process. They are never used to satisfy the assertions below.
		lookup.releaseOnce.Do(func() { close(lookup.release) })
		foreground.releaseOnce.Do(func() { close(foreground.release) })
		cancelRequest()
		cancelStart()
		closeRuntime()
		awaitPluginSignal(t, closed, "shutdown cleanup")
	})
	query := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "running"})
	if required, err := plugins.RequiresState(repo, commit.Result.NewCommit, query); err != nil || !required {
		t.Fatalf("fixture is not a real Bound State query: required=%v error=%v", required, err)
	}
	plugins.SetObservationSource(sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "shutdown-worker"}, RequestID: "shutdown-worker-request"}})
	if err := plugins.Start(startContext); err != nil {
		t.Fatal(err)
	}
	var observation liveaccess.StateLookupRequest
	select {
	case observation = <-lookup.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual Controller startup did not reach the Bound State source")
	}
	if observation.Binding.Repository != repo.ID() || observation.Binding.DeclarationCommit != commit.Result.NewCommit || observation.Binding.Address.ObjectID != "job/shutdown" || observation.Binding.Address.AspectName != "runtime" || observation.Binding.Mode != objectmodel.BindingState || observation.Binding.DeclarationDigest == "" || observation.SchemaRef != "schema/job.runtime" || observation.Origin != "https://scheduler.example" || observation.Identity.Principal != "shutdown-worker" || observation.RequestID != "shutdown-worker-request" {
		t.Fatalf("background observation lost its actual fixed Binding or identity: %#v", observation)
	}
	finished := make(chan error, 1)
	go func() { finished <- plugins.CatchUpRepository(requestContext, repo.ID()) }()
	awaitPluginSignal(t, foreground.entered, "accepted foreground CatchUpRepository")
	select {
	case <-foreground.drained:
		t.Fatal("foreground CatchUp did not wait for the background Controller lock")
	case <-time.After(30 * time.Millisecond):
	}
	retained := shutdownRetainedEngines(t, f.pool)
	closeRuntime()
	awaitPluginStopped(t, plugins, repo, commit.Result.NewCommit)
	select {
	case <-lookup.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel the background State lookup while accepted CatchUpRepository waited")
	}
	awaitPluginSignal(t, foreground.drained, "actual foreground CatchUp after background cancellation")
	select {
	case <-closed:
		t.Fatal("Close released the runtime before accepted foreground work drained")
	default:
	}
	assertShutdownEnginesOpen(t, retained)
	assertShutdownEnginesOpen(t, shutdownRetainedEngines(t, f.pool))
	revision, ready := queries.StateView(repo.ID(), commit.Result.NewCommit)
	if !ready || revision == "" {
		t.Fatal("accepted foreground CatchUp did not publish the actual State projection")
	}
	result, err := queries.SearchStateAtRevisionContext(context.Background(), repo, commit.Result.NewCommit, revision, query)
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Address.ObjectID != "job/shutdown" || result.Hits[0].Object.Commit != commit.Result.NewCommit || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("draining task did not preserve the real fixed State query: %#v %v", result, err)
	}
	value := result.Hits[0].Object.Value.(map[string]any)
	if value["runtime"].(map[string]any)["status"] != "running" {
		t.Fatalf("State query did not return the observed body: %#v", value)
	}
	foreground.releaseOnce.Do(func() { close(foreground.release) })
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("Close canceled the accepted foreground context: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("accepted foreground CatchUp did not drain")
	}
	awaitPluginSignal(t, closed, "runtime close after actual State work drained")
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	assertPluginEngineCloseCounts(t, f.pool, 1)
	if err := plugins.Close(); err != nil {
		t.Fatal(err)
	}
	assertPluginEngineCloseCounts(t, f.pool, 1)
}

// This adapter only signals entry after the registry has acquired its task
// lease, and holds the completed real task while shutdown drainage is checked.
// Controller.Start/Close and every control operation remain the actual Entry's.
type shutdownCatchUp struct {
	*indexcontrol.Controller
	entered     chan struct{}
	drained     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func shutdownRetainedEngines(t *testing.T, pool *pluginEnginePool) []*pluginPhysicalEngine {
	t.Helper()
	pool.mu.Lock()
	defer pool.mu.Unlock()
	var retained []*pluginPhysicalEngine
	for _, engine := range pool.handles {
		// Read-only projection checks may already have released a one-call
		// handle. The runtime must retain its live and State handles until drain.
		if engine.closed.Load() == 0 {
			retained = append(retained, engine)
		}
	}
	if len(retained) == 0 {
		t.Fatal("actual Controller work did not retain a physical engine")
	}
	return retained
}

func assertShutdownEnginesOpen(t *testing.T, engines []*pluginPhysicalEngine) {
	t.Helper()
	for _, engine := range engines {
		if count := engine.closed.Load(); count != 0 {
			t.Fatalf("inflight physical resource %s/%s closed %d times before drain", engine.namespace, engine.repository, count)
		}
	}
}

func (c *shutdownCatchUp) CatchUp(ctx context.Context) error {
	close(c.entered)
	err := c.Controller.CatchUp(ctx)
	close(c.drained)
	<-c.release
	return err
}

type shutdownStateLookup struct {
	entered     chan liveaccess.StateLookupRequest
	canceled    chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	calls       atomic.Int64
}

func (s *shutdownStateLookup) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	if s.calls.Add(1) == 1 {
		s.entered <- request
		select {
		case <-ctx.Done():
			close(s.canceled)
			return liveaccess.StateObservation{}, ctx.Err()
		case <-s.release:
		}
	}
	return liveaccess.StateObservation{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{
		BindingGeneration: "shutdown-generation", Consistency: objectmodel.ObservationRepeatable,
		SourceRevision: "shutdown-revision", ObservedAt: "2026-10-14T00:00:00Z",
	}}, nil
}

func shutdownStateEntry(t *testing.T, f *pluginFixture) consumerplugin.IndexPluginEntry {
	t.Helper()
	return declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"), Lookup: f.lookup, Datasets: f.cat,
		Engine: func(namespace string) index.EngineOpener {
			open := f.pool.opener(namespace)
			return func(directory string, repository kernel.RepositoryID) (index.Engine, error) {
				engine, err := open(directory, repository)
				if err != nil {
					return nil, err
				}
				return &shutdownStreamingEngine{engine.(*pluginPhysicalEngine)}, nil
			}
		},
	})
}

type shutdownStreamingEngine struct{ *pluginPhysicalEngine }

func (e *shutdownStreamingEngine) BeginRebuild(meta index.Meta) (index.RebuildSession, error) {
	return &shutdownRebuildSession{engine: e.pluginPhysicalEngine, meta: meta}, nil
}

type shutdownRebuildSession struct {
	engine *pluginPhysicalEngine
	meta   index.Meta
	docs   []index.CompiledDoc
}

func (s *shutdownRebuildSession) Append(docs []index.CompiledDoc) error {
	s.docs = append(s.docs, docs...)
	return nil
}
func (s *shutdownRebuildSession) Commit() error       { return s.engine.Rebuild(s.docs, s.meta) }
func (*shutdownRebuildSession) Abort(err error) error { return err }

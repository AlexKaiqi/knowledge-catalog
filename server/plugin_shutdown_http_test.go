package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	indexcontrol "kc/declarativeindex/control"
	declarative "kc/declarativeindex/plugin"
	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// HTTP shutdown must cancel actual background State work before waiting for a
// sync request that needs its Controller lock. Registration, compilation,
// State publication and canonical reads use the real Entry and object stack.
func TestHTTPPluginShutdownCancelsStateBeforeDrainingAcceptedSync(t *testing.T) {
	exerciseHTTPPluginShutdown(t, false)
}

func TestHTTPBeginShutdownCancelsStateThenDrainsBeforeResourceClose(t *testing.T) {
	exerciseHTTPPluginShutdown(t, true)
}

func exerciseHTTPPluginShutdown(t *testing.T, beginThenDrain bool) {
	t.Helper()
	source := testkit.NewSetup(t, "kr://shutdown/http-source")
	commit, err := source.Writer.Commit("seed-http-shutdown", objectmodel.CommitChangeSet{
		TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: source.RootCommitID, ExpectedTargetCommit: source.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "Job:shutdown"}, Value: map[string]any{"name": "shutdown"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime", "origin": "https://scheduler.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.NewRegistry()
	if err := store.Add(source.Repo.Snapshot()); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(store)
	repo, err := rd.Require(source.RepositoryID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	pool := &httpShutdownPool{stores: map[string]*httpShutdownStorage{}}
	entry := declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"),
		Engine: func(string) index.EngineOpener { return pool.open }, Datasets: testkit.OpenDatasets(t, store),
		Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return rd.Require(id, kernel.ErrKnowledgeRefUnresolved)
		},
	})
	open := entry.Open
	var foreground *httpShutdownController
	var stops atomic.Int64
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil {
			controller := contribution.Maintenance.(*indexcontrol.Controller)
			controller.SetReconcileInterval(time.Hour)
			foreground = &httpShutdownController{Controller: controller, entered: make(chan struct{}), drained: make(chan struct{}), release: make(chan struct{})}
			contribution.Maintenance = foreground
			stop := contribution.Stop
			contribution.Stop = func() { stops.Add(1); stop() }
		}
		return contribution, err
	}
	plugins, err := consumerplugin.OpenIndexPlugins(consumerplugin.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]consumerplugin.RepositoryPluginConfig{
		source.RepositoryID: {Version: 1, Plugins: []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "http-shutdown"}}},
	}}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	lookup := &httpShutdownState{entered: make(chan liveaccess.StateLookupRequest, 1), canceled: make(chan struct{}), release: make(chan struct{})}
	startContext, cancelStart := context.WithCancel(context.Background())
	t.Cleanup(func() {
		lookup.releaseOnce.Do(func() { close(lookup.release) })
		foreground.releaseOnce.Do(func() { close(foreground.release) })
		cancelStart()
		_ = plugins.Close()
	})
	plugins.SetObservationSource(sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "http-shutdown-worker"}}})
	if err := plugins.Start(startContext); err != nil {
		t.Fatal(err)
	}
	select {
	case observed := <-lookup.entered:
		if observed.Binding.DeclarationCommit != commit.Result.NewCommit || observed.Binding.Repository != source.RepositoryID || observed.Binding.Address.ObjectID != "Job:shutdown" || observed.Binding.Address.AspectName != "runtime" || observed.Identity.Principal != "http-shutdown-worker" {
			t.Fatalf("background did not use the actual fixed Binding: %#v", observed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("actual Controller did not enter background Bound State lookup")
	}
	dir := t.TempDir()
	if err := inv.WriteAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{{ID: "sync", Principal: "maintainer", Repo: string(source.RepositoryID), Actions: []string{"projection.manage"}}}}); err != nil {
		t.Fatal(err)
	}
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "http-plugin-shutdown-test"})
	if err != nil {
		t.Fatal(err)
	}
	ws := &Home{Dir: dir, Store: store, Reader: rd, Index: plugins, Plugins: plugins}
	// This fixture supplies an already genuinely activated consumer root. Cold
	// construction and activation are independently checked by their owner tests.
	facade := &httpFacade{home: dir, readHome: ws, workersStarted: true, runtime: runtime, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	handler := &ManagedHTTPHandler{Handler: mux, runtime: runtime, closeHome: facade.closeReadHome, stopHome: facade.stopReadHome}
	var network *httptest.Server
	if beginThenDrain {
		network = httptest.NewServer(handler)
		t.Cleanup(network.Close)
	}
	closed, finished := make(chan error, 1), make(chan struct{})
	drained := make(chan error, 1)
	var transportErr error
	requestContext, cancelRequest := context.WithCancel(context.Background())
	var closeOnce sync.Once
	closeRuntime := func() { closeOnce.Do(func() { go func() { closed <- handler.Close(); close(closed) }() }) }
	t.Cleanup(func() {
		// Releases only prevent a broken shutdown implementation from hanging
		// the process. They do not satisfy the cancellation/drain assertions.
		lookup.releaseOnce.Do(func() { close(lookup.release) })
		foreground.releaseOnce.Do(func() { close(foreground.release) })
		cancelRequest()
		cancelStart()
		closeRuntime()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("HTTP shutdown cleanup did not finish the request")
		}
		select {
		case err := <-closed:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("HTTP shutdown cleanup did not release its root")
		}
	})
	body, _ := json.Marshal(map[string]any{"repository": source.RepositoryID, "commit": commit.Result.NewCommit})
	request := httptest.NewRequest(http.MethodPost, "/operations/v1/projections:sync", strings.NewReader(string(body))).WithContext(requestContext)
	if network != nil {
		request, err = http.NewRequestWithContext(requestContext, http.MethodPost, network.URL+"/operations/v1/projections:sync", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
	}
	request.Header.Set("X-Kc-As", "maintainer")
	response := httptest.NewRecorder()
	go func() {
		defer close(finished)
		if network == nil {
			handler.ServeHTTP(response, request)
			return
		}
		var result *http.Response
		result, transportErr = network.Client().Do(request)
		if transportErr == nil {
			defer result.Body.Close()
			var raw []byte
			raw, transportErr = io.ReadAll(result.Body)
			response.Code = result.StatusCode
			_, _ = response.Body.Write(raw)
		}
	}()
	httpShutdownSignal(t, foreground.entered, "actual HTTP foreground CatchUp")
	select {
	case <-foreground.drained:
		t.Fatal("foreground sync did not wait for the actual background Controller lock")
	case <-time.After(25 * time.Millisecond):
	}
	pool.mu.Lock()
	var retained []*httpShutdownEngine
	for _, engine := range pool.handles {
		if engine.closed.Load() == 0 {
			retained = append(retained, engine)
		}
	}
	pool.mu.Unlock()
	if len(retained) == 0 {
		t.Fatal("actual background work retained no physical resources")
	}
	if beginThenDrain {
		handler.BeginShutdown()
		denied := httptest.NewRecorder()
		late := httptest.NewRequest(http.MethodPost, "/operations/v1/projections:sync", strings.NewReader(string(body)))
		late.Header.Set("X-Kc-As", "maintainer")
		handler.ServeHTTP(denied, late)
		if denied.Code == http.StatusOK || !strings.Contains(denied.Body.String(), string(kernel.ErrPreconditionFailed)) {
			t.Fatalf("BeginShutdown accepted a new typed request: HTTP %d %s", denied.Code, denied.Body.String())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go func() { drained <- network.Config.Shutdown(ctx) }()
		select {
		case err := <-drained:
			t.Fatalf("HTTP server drained before its accepted sync returned: %v", err)
		case <-time.After(25 * time.Millisecond):
		}
	} else {
		closeRuntime()
	}
	select {
	case <-lookup.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP Close waited for its invocation lock without canceling background State work")
	}
	httpShutdownSignal(t, foreground.drained, "foreground State publication after background cancellation")
	select {
	case err := <-closed:
		t.Fatalf("HTTP shutdown released resources before the accepted sync drained: %v", err)
	default:
	}
	if requestContext.Err() != nil {
		t.Fatal("shutdown canceled the foreground request context")
	}
	for _, engine := range retained {
		if count := engine.closed.Load(); count != 0 {
			t.Fatalf("retained physical resource closed %d times before HTTP drain", count)
		}
	}
	query := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "running"})
	revision, ready := plugins.StateView(repo.ID(), commit.Result.NewCommit)
	if !ready || revision == "" {
		t.Fatal("accepted HTTP sync did not publish its actual State view")
	}
	result, err := plugins.SearchStateAtRevisionContext(context.Background(), repo, commit.Result.NewCommit, revision, query)
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Knowledge.Commit != commit.Result.NewCommit || result.Hits[0].Knowledge.Address.ObjectID != "Job:shutdown" || len(result.Hits[0].Version.Observations) != 1 || result.Hits[0].Knowledge.Value.(map[string]any)["runtime"].(map[string]any)["status"] != "running" {
		t.Fatalf("draining HTTP work lost its fixed canonical State query: %#v %v", result, err)
	}
	foreground.releaseOnce.Do(func() { close(foreground.release) })
	httpShutdownSignal(t, finished, "accepted HTTP sync response")
	if transportErr != nil {
		t.Fatal(transportErr)
	}
	var synced index.IndexSync
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &synced) != nil || synced.Mode != index.IndexModeReady || synced.Repository != source.RepositoryID || synced.BasisCommit != commit.Result.NewCommit || synced.AccessDigest == "" || synced.ObjectCount != 1 {
		t.Fatalf("accepted sync failed during shutdown: HTTP %d %s", response.Code, response.Body.String())
	}
	if beginThenDrain {
		select {
		case err := <-drained:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("HTTP server did not drain after accepted State sync completed")
		}
		for _, engine := range retained {
			if count := engine.closed.Load(); count != 0 {
				t.Fatalf("physical resource closed %d times before final Close", count)
			}
		}
		closeRuntime()
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP runtime did not release resources after the accepted sync")
	}
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	if count := stops.Load(); count != 1 {
		t.Fatalf("actual contribution stopped %d times, want once", count)
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, handle := range pool.handles {
		if count := handle.closed.Load(); count != 1 {
			t.Fatalf("physical handle closed %d times, want one", count)
		}
	}
}

type httpShutdownController struct {
	*indexcontrol.Controller
	entered, drained, release chan struct{}
	releaseOnce               sync.Once
}

func (c *httpShutdownController) CatchUp(ctx context.Context) error {
	close(c.entered)
	err := c.Controller.CatchUp(ctx)
	close(c.drained)
	<-c.release
	return err
}

type httpShutdownState struct {
	entered     chan liveaccess.StateLookupRequest
	canceled    chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	calls       atomic.Int64
}

func (s *httpShutdownState) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
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
		BindingGeneration: "http-shutdown-generation", Consistency: objectmodel.ObservationRepeatable,
		SourceRevision: "http-shutdown-revision", ObservedAt: "2026-10-14T00:00:00Z",
	}}, nil
}

func httpShutdownSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("did not reach " + description)
	}
}

// A replaceable physical medium with one shared store per real engine key.
// Superset candidates are residual-checked by the actual Index.
type httpShutdownStorage struct {
	mu   sync.Mutex
	meta index.Meta
	docs map[objectmodel.ObjectID]index.CompiledDoc
}
type httpShutdownPool struct {
	mu      sync.Mutex
	stores  map[string]*httpShutdownStorage
	handles []*httpShutdownEngine
}
type httpShutdownEngine struct {
	store  *httpShutdownStorage
	closed atomic.Int64
}

func (p *httpShutdownPool) open(directory string, repo kernel.RepositoryID) (index.Engine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := directory + "\x00" + string(repo)
	if p.stores[key] == nil {
		p.stores[key] = &httpShutdownStorage{docs: map[objectmodel.ObjectID]index.CompiledDoc{}}
	}
	engine := &httpShutdownEngine{store: p.stores[key]}
	p.handles = append(p.handles, engine)
	return engine, nil
}
func (e *httpShutdownEngine) LoadMeta() (index.Meta, error) {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	return e.store.meta, nil
}
func (e *httpShutdownEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	e.store.docs = map[objectmodel.ObjectID]index.CompiledDoc{}
	for _, doc := range docs {
		e.store.docs[doc.ObjectID] = doc
	}
	e.store.meta = meta
	return nil
}
func (e *httpShutdownEngine) Apply(docs []index.CompiledDoc, deletes []objectmodel.ObjectID, meta index.Meta) error {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	for _, id := range deletes {
		delete(e.store.docs, id)
	}
	for _, doc := range docs {
		e.store.docs[doc.ObjectID] = doc
	}
	e.store.meta = meta
	return nil
}
func (e *httpShutdownEngine) Count() (int, error) {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	return len(e.store.docs), nil
}
func (e *httpShutdownEngine) Close() error { e.closed.Add(1); return nil }
func (*httpShutdownEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeSuperset, Coverage: 1}
}
func (e *httpShutdownEngine) Retrieve(request index.RetrieveRequest) (index.CandidatePage, error) {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	page := index.CandidatePage{Exhausted: true}
	for id := range e.store.docs {
		page.Candidates = append(page.Candidates, index.CandidateRef{Repository: request.Spec.Repository, ObjectID: id, Basis: e.store.meta.Basis})
	}
	sort.Slice(page.Candidates, func(i, j int) bool { return page.Candidates[i].ObjectID < page.Candidates[j].ObjectID })
	return page, nil
}
func (e *httpShutdownEngine) BeginRebuild(meta index.Meta) (index.RebuildSession, error) {
	return &httpShutdownBuild{engine: e, meta: meta}, nil
}

type httpShutdownBuild struct {
	engine *httpShutdownEngine
	meta   index.Meta
	docs   []index.CompiledDoc
}

func (b *httpShutdownBuild) Append(docs []index.CompiledDoc) error {
	b.docs = append(b.docs, docs...)
	return nil
}
func (b *httpShutdownBuild) Commit() error       { return b.engine.Rebuild(b.docs, b.meta) }
func (*httpShutdownBuild) Abort(err error) error { return err }

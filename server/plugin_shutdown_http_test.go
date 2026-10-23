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
	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/internal/bindingfixture"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	liveplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// HTTP shutdown must cancel actual background State work before waiting for a
// sync request that needs its State build lock. Registration, compilation,
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
	descriptor := bindingfixture.Descriptor("https://scheduler.example")
	descriptor["identity"] = string(objectmodel.IdentityShared)
	commit, err := source.Writer.Commit("seed-http-shutdown", objectmodel.CommitChangeSet{
		TargetRepository: source.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: source.RootCommitID, ExpectedTargetCommit: source.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/shutdown", AspectName: "definition"}, Value: map[string]any{"name": "shutdown"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime",
				"fields": map[string]any{"status": map[string]any{"type": "string"}},
			}},
			testkit.IndexField("schema/job.runtime", "status", "keyword"),
			bindingfixture.PutDescriptor("resource/scheduler", descriptor),
			bindingfixture.PutAspect("job/shutdown", "runtime", "schema/job.runtime", "resource/scheduler"),
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
	repo, err := rd.Require(source.RepositoryID, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenBoundDatasets(t, store)
	if _, err := cat.DefineDataset("shutdown", 1, []dataset.DatasetSource{{Repository: source.RepositoryID, Selector: snapshot.DefaultRef, BaseRev: string(commit.Result.NewCommit), Path: dataset.MountPath("jobs")}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := cat.ResolveDataset("shutdown")
	if err != nil {
		t.Fatal(err)
	}
	pin, memberLookup := testkit.DatasetPin(resolved), rd.Lookup(cat.Require)
	pool := &httpShutdownPool{stores: map[string]*httpShutdownStorage{}}
	entry := declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"),
		Engine: func(string) index.EngineOpener { return pool.open }, Datasets: cat,
		// Explicit preparation builds the accepted release, so the assembly has
		// to be handed the one conversion to a consumer basis that the real
		// deployment provides; without it the entry installs no build entry at
		// all and preparation is refused instead of reaching the State refresh.
		DatasetPin: testkit.DatasetPin,
		Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return rd.Require(id, kernel.ErrObjectRefUnresolved)
		},
	})
	open := entry.Open
	var foreground *httpShutdownQueries
	var stops atomic.Int64
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil {
			controller := contribution.Maintenance.(*indexcontrol.Controller)
			controller.SetReconcileInterval(time.Hour)
			foreground = &httpShutdownQueries{Index: contribution.Queries.(*indeximplementation.Index), entered: make(chan struct{}), drained: make(chan struct{}), release: make(chan struct{})}
			contribution.Queries = foreground
			stop := contribution.Stop
			contribution.Stop = func() { stops.Add(1); stop() }
		}
		return contribution, err
	}
	installed := consumerplugin.PluginsConfig{"declarative-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{"http-shutdown": {Catalog: declarative.Catalog}}}}
	plugins, err := consumerplugin.OpenIndexPlugins(installed, consumerplugin.PluginScope{Repositories: []kernel.RepositoryID{source.RepositoryID}}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.PrepareDatasetScope(context.Background(), memberLookup, pin); err != nil {
		t.Fatal(err)
	}
	lookup := &httpShutdownState{entered: make(chan liveaccess.StateLookupRequest, 1), canceled: make(chan struct{}), release: make(chan struct{})}
	access, err := liveservice.OpenPlugins(consumerplugin.PluginsConfig{"liveaccess": {Instances: map[string]consumerplugin.PluginInstanceConfig{"main": {Protocol: "resource-access/v1"}}}}, []kernel.RepositoryID{source.RepositoryID}, map[string]liveservice.PluginEntry{
		"liveaccess": liveplugin.Entry(liveplugin.Options{OpenLookup: func(consumerplugin.PluginScope) (liveaccess.StateLookup, error) { return lookup, nil }}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = access.Close() })
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
		if observed.Binding.DeclarationCommit != commit.Result.NewCommit || observed.Binding.Repository != source.RepositoryID || observed.Binding.Address.ObjectID != "job/shutdown" || observed.Binding.Address.AspectName != "runtime" || observed.Identity.Principal != "http-shutdown-worker" {
			t.Fatalf("background did not use the actual fixed Binding: %#v", observed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("actual Controller did not enter background Bound State lookup")
	}
	dir := t.TempDir()
	if err := inv.WriteAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{{ID: "sync", Principal: "maintainer", Dataset: "shutdown", Actions: []string{"dataset.manage", "index.manage"}}}}); err != nil {
		t.Fatal(err)
	}
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "http-plugin-shutdown-test"})
	if err != nil {
		t.Fatal(err)
	}
	ws := &Home{Dir: dir, Store: store, Reader: rd, Datasets: cat, Index: plugins, Plugins: plugins, LiveAccess: access}
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
	body, _ := json.Marshal(map[string]any{"dataset": "shutdown", "revision": 1})
	request := httptest.NewRequest(http.MethodPost, "/operations/v1/indexes:prepare", strings.NewReader(string(body))).WithContext(requestContext)
	if network != nil {
		request, err = http.NewRequestWithContext(requestContext, http.MethodPost, network.URL+"/operations/v1/indexes:prepare", strings.NewReader(string(body)))
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
	httpShutdownSignal(t, foreground.entered, "actual HTTP foreground Dataset State refresh")
	select {
	case <-foreground.drained:
		t.Fatal("foreground sync did not wait for the actual background State build lock")
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
		late := httptest.NewRequest(http.MethodPost, "/operations/v1/indexes:prepare", strings.NewReader(string(body)))
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
	if foreground.failure != nil {
		t.Fatalf("foreground Dataset State publication failed: %v", foreground.failure)
	}
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
	result, err := plugins.SearchDataset(context.Background(), memberLookup, pin, query)
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Commit != commit.Result.NewCommit || result.Hits[0].Object.Address.ObjectID != "job/shutdown" || len(result.Hits[0].Version.Observations) != 1 || result.Hits[0].Object.Value.(map[string]any)["runtime"].(map[string]any)["status"] != "running" {
		t.Fatalf("draining HTTP work lost its fixed canonical State query: %#v %v", result, err)
	}
	revision := result.SearchView.IndexRevisions[repo.ID()]
	if revision == "" || result.Dataset == nil || result.Dataset.ID != "shutdown" || result.Dataset.Revision != 1 {
		t.Fatalf("accepted HTTP sync did not publish its actual Dataset State view: %#v", result)
	}
	foreground.releaseOnce.Do(func() { close(foreground.release) })
	httpShutdownSignal(t, finished, "accepted HTTP sync response")
	if transportErr != nil {
		t.Fatal(transportErr)
	}
	var synced struct {
		Dataset  string `json:"dataset"`
		Revision int    `json:"revision"`
		Ready    bool   `json:"ready"`
		Members  []struct {
			Repository kernel.RepositoryID `json:"repository"`
			Commit     kernel.CommitID     `json:"commit"`
		} `json:"members"`
		// Basis is the corpus this revision froze. Its mode records how the
		// corpus was built (rebuild or incremental), so readiness is the
		// separate top-level answer rather than a word inside the basis.
		Basis *index.IndexBasisFacts `json:"basis"`
		// Preparation reports the observation it just published as
		// "observation": a revision whose corpus only now exists has no other
		// place for it, and the pin already fixes the basis commit.
		Observation *index.StateSync `json:"observation"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &synced) != nil || synced.Dataset != "shutdown" || synced.Revision != 1 || !synced.Ready || len(synced.Members) != 1 || synced.Members[0].Repository != source.RepositoryID || synced.Members[0].Commit != commit.Result.NewCommit || synced.Basis == nil || synced.Basis.AccessDigest == "" || synced.Basis.ObjectCount != 3 /* job, its ResourceDescriptor and the index declaration member */ || synced.Observation == nil || synced.Observation.Revision != revision || synced.Observation.ObservationDigest == "" {
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

type httpShutdownQueries struct {
	*indeximplementation.Index
	entered, drained, release chan struct{}
	releaseOnce               sync.Once
	failure                   error
}

func (c *httpShutdownQueries) RefreshDatasetState(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, source index.ObservationSource) (index.StateSync, error) {
	close(c.entered)
	result, err := c.Index.RefreshDatasetState(ctx, lookup, pin, source)
	c.failure = err
	close(c.drained)
	<-c.release
	return result, err
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

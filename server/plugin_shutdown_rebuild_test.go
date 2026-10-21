package server

import (
	"context"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	indexcontrol "kc/declarativeindex/control"
	declarative "kc/declarativeindex/plugin"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	"kc/object/reader"
	"kc/snapshot"
)

// gatedScanRepository serves the background rebuild one object per page and
// holds the first page until the test releases it, i.e. a rebuild that is
// mid-way through reading objects when shutdown begins.
type gatedScanRepository struct {
	*testkit.ObjectRepository
	entered chan struct{}
	release chan struct{}
	pages   int
}

func (r *gatedScanRepository) ScanSnapshotPage(commit kernel.CommitID, request objectmaintenance.ScanRequest) (objectmaintenance.ScanPage, error) {
	r.pages++
	if r.pages == 1 {
		close(r.entered)
		<-r.release
	}
	request.Limit = 1
	return r.ObjectRepository.ScanSnapshotPage(commit, request)
}

func TestHTTPBeginShutdownStopsInFlightRebuildWithoutAdvancingMeta(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://shutdown/http-rebuild")
	ops := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexText("schema/article", "body"),
	}
	for _, id := range []string{"article/a", "article/b", "article/c", "article/d"} {
		ops = append(ops, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, SchemaRef: "schema/article", Value: map[string]any{"body": id}})
	}
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	if _, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: ops}); err != nil {
		t.Fatal(err)
	}
	gated := &gatedScanRepository{ObjectRepository: repo, entered: make(chan struct{}), release: make(chan struct{})}
	store := snapshot.NewRegistry()
	rd := reader.NewReader(store)
	cat := testkit.OpenDatasets(t, store)
	pool := &httpShutdownPool{stores: map[string]*httpShutdownStorage{}}
	entry := declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"),
		Engine: func(string) index.EngineOpener { return pool.open }, Datasets: cat,
		Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			if id != repo.ID() {
				return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "unknown repository %s", id)
			}
			return gated, nil
		},
	})
	open := entry.Open
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil {
			contribution.Maintenance.(*indexcontrol.Controller).SetReconcileInterval(time.Hour)
		}
		return contribution, err
	}
	installed := consumerplugin.PluginsConfig{"declarative-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{"http-rebuild": {Catalog: declarative.Catalog}}}}
	plugins, err := consumerplugin.OpenIndexPlugins(installed, consumerplugin.PluginScope{Repositories: []kernel.RepositoryID{repo.ID()}}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			close(gated.release)
		}
	}
	t.Cleanup(func() { release(); _ = plugins.Close() })
	if err := plugins.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gated.entered:
	case <-time.After(5 * time.Second):
		targets, err := plugins.Targets()
		buf := make([]byte, 1<<20)
		n := goruntime.Stack(buf, true)
		t.Fatalf("background rebuild never scanned objects: targets=%#v err=%v\n%s", targets, err, buf[:n])
	}

	runtime, err := telemetry.New(telemetry.Config{ServiceName: "http-rebuild-shutdown-test"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ws := &Home{Dir: dir, Store: store, Reader: rd, Datasets: cat, Index: plugins, Plugins: plugins}
	facade := &httpFacade{home: dir, readHome: ws, workersStarted: true, runtime: runtime, options: HTTPServerOptions{AuthMode: "local"}}
	handler := &ManagedHTTPHandler{Handler: nil, runtime: runtime, closeHome: facade.closeReadHome, stopHome: facade.stopReadHome}

	begun := make(chan struct{})
	go func() { handler.BeginShutdown(); close(begun) }()
	select {
	case <-begun:
		t.Fatal("BeginShutdown returned before the in-flight object read finished")
	case <-time.After(25 * time.Millisecond):
	}
	// The single in-flight object read completes; the rebuild must observe the
	// cancellation at the next object instead of walking the whole Snapshot.
	release()
	select {
	case <-begun:
	case <-time.After(5 * time.Second):
		t.Fatal("BeginShutdown did not stop background rebuild within a bounded time")
	}
	targets, err := plugins.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Status != index.TargetPending || targets[0].LastError != "" {
		t.Fatalf("interrupted rebuild must leave its durable target pending: %#v", targets)
	}
	closed := make(chan error, 1)
	go func() { closed <- handler.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP Close did not finish after maintenance stopped")
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for key, storage := range pool.stores {
		if storage.meta.Basis != "" || len(storage.docs) != 0 {
			t.Fatalf("interrupted rebuild advanced engine %q: meta=%+v docs=%d", key, storage.meta, len(storage.docs))
		}
	}
	if gated.pages > 1 {
		t.Fatalf("rebuild kept reading %d pages after shutdown", gated.pages)
	}
}

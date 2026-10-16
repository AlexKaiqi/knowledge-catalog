package server

import (
	"context"
	"encoding/json"
	"errors"
	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	apphome "kc/home"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type httpSearchCancellationEngine struct {
	meta    index.Meta
	entered chan struct{}
	cause   error
}

func (e *httpSearchCancellationEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}
func (e *httpSearchCancellationEngine) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	return index.CandidatePage{}, errors.New("legacy Retrieve lost HTTP context")
}
func (e *httpSearchCancellationEngine) RetrieveContext(ctx context.Context, _ index.RetrieveRequest) (index.CandidatePage, error) {
	close(e.entered)
	<-ctx.Done()
	e.cause = ctx.Err()
	return index.CandidatePage{}, ctx.Err()
}
func (e *httpSearchCancellationEngine) LoadMeta() (index.Meta, error) { return e.meta, nil }
func (e *httpSearchCancellationEngine) Rebuild(_ []index.CompiledDoc, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *httpSearchCancellationEngine) Apply(_ []index.CompiledDoc, _ []objectmodel.ObjectID, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *httpSearchCancellationEngine) Count() (int, error) { return 0, nil }
func (e *httpSearchCancellationEngine) Close() error        { return nil }

func TestHTTPSearchCancellationReachesProvider(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/cancel")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}, testkit.IndexText("schema/item", "body")}})
	if err != nil {
		t.Fatal(err)
	}
	engine := &httpSearchCancellationEngine{entered: make(chan struct{})}
	idx := indeximplementation.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	defer idx.Close()
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenDatasets(t, registry)
	if _, err := cat.DefineDataset("cancel-search", 1, []dataset.DatasetSource{{Repository: repo.ID(), Selector: snapshot.DefaultRef, BaseRev: string(commit), Path: dataset.MountPath("items")}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := cat.ResolveDataset("cancel-search")
	if err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	if _, err := idx.PrepareDataset(context.Background(), rd.Lookup(cat.Require), testkit.DatasetPin(resolved)); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ws := &Home{Dir: dir, Store: registry, Reader: rd, Index: idx, Datasets: cat, File: apphome.HomeFile{Registry: apphome.HomeRegistry{ID: "kr://acme/catalog"}}}
	if err := inv.WriteAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{{ID: "search", Principal: "reader", Dataset: "cancel-search", Actions: []string{"file.read"}}}}); err != nil {
		t.Fatal(err)
	}
	facade := &httpFacade{home: dir, readHome: ws, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	body, _ := json.Marshal(map[string]any{"dataset": "cancel-search", "revision": 1, "query": "runbook"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/object/v1/search", strings.NewReader(string(body))).WithContext(ctx)
	request.Header.Set("X-Kc-As", "reader")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); mux.ServeHTTP(response, request) }()
	select {
	case <-engine.entered:
		cancel()
	case <-done:
		t.Fatalf("provider not reached: %d %s", response.Code, response.Body)
	case <-time.After(time.Second):
		t.Fatal("provider not reached")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("HTTP cancellation did not finish")
	}
	if !errors.Is(engine.cause, context.Canceled) || response.Code == http.StatusOK {
		t.Fatalf("cancellation: cause=%v status=%d body=%s", engine.cause, response.Code, response.Body)
	}
}

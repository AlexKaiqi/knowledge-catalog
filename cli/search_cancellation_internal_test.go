package cli

import (
	"context"
	"encoding/json"
	"errors"
	"kc/controlplane"
	apphome "kc/home"
	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
	"kc/snapshot"
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
func (e *httpSearchCancellationEngine) Apply(_ []index.CompiledDoc, _ []knowledge.ObjectID, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *httpSearchCancellationEngine) Count() (int, error) { return 0, nil }
func (e *httpSearchCancellationEngine) Close() error        { return nil }

func TestHTTPSearchCancellationReachesProvider(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/cancel")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	engine := &httpSearchCancellationEngine{entered: make(chan struct{})}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	defer idx.Close()
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ws := &Home{Dir: dir, Store: registry, Reader: reader.NewReader(registry), Index: idx, File: apphome.HomeFile{Catalogs: []apphome.HomeCatalog{{ID: "kr://acme/catalog"}}}, Controls: map[string]controlplane.ControlState{}}
	if err := WriteAllow(dir, AllowFile{Version: allowVersion, Rules: []AllowRule{{ID: "search", Principal: "reader", Repo: string(repo.ID()), Actions: []string{"knowledge.search"}}}}); err != nil {
		t.Fatal(err)
	}
	facade := &httpFacade{home: dir, readHome: ws, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	body, _ := json.Marshal(map[string]any{"repository": repo.ID(), "commit": commit, "query": "runbook"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/knowledge/v1/search", strings.NewReader(string(body))).WithContext(ctx)
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

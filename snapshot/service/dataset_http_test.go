package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

type datasetHTTPSource struct {
	id   kernel.RepositoryID
	head kernel.CommitID
}

func (s *datasetHTTPSource) ID() kernel.RepositoryID                 { return s.id }
func (s *datasetHTTPSource) Head(string) (kernel.CommitID, error)    { return s.head, nil }
func (s *datasetHTTPSource) GetRef(string) (kernel.CommitID, bool)   { return s.head, true }
func (s *datasetHTTPSource) HasCommit(id kernel.CommitID) bool       { return id == "c1" || id == "c2" }
func (s *datasetHTTPSource) CreateRef(string, kernel.CommitID) error { return nil }
func (s *datasetHTTPSource) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", fmt.Errorf("unused")
}
func (s *datasetHTTPSource) Archived() bool { return false }
func (s *datasetHTTPSource) Archive() error { return nil }

type datasetHTTPCatalogs struct{ catalog *dataset.Catalog }

func (p datasetHTTPCatalogs) CatalogIDs() []string { return []string{"A", "B"} }
func (p datasetHTTPCatalogs) OpenCatalog(id string) (*dataset.Catalog, error) {
	if id != "A" {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "Catalog missing")
	}
	return p.catalog, nil
}

type datasetHTTPRules struct{ rules []access.Rule }

func (s *datasetHTTPRules) ReadRules(context.Context) ([]access.Rule, error) { return s.rules, nil }
func (s *datasetHTTPRules) UpdateRules(ctx context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	rules, err := update(s.rules)
	if err == nil {
		s.rules = rules
	}
	return err
}

type datasetHTTPFixture struct {
	catalog   *dataset.Catalog
	source    *datasetHTTPSource
	rules     *datasetHTTPRules
	resources service.Resources
}

func newDatasetHTTPFixture(t *testing.T) *datasetHTTPFixture {
	t.Helper()
	store := snapshot.NewRegistry()
	source := &datasetHTTPSource{id: "kr://files/source", head: "c1"}
	if err := store.Add(source); err != nil {
		t.Fatal(err)
	}
	registry, err := dataset.NewRegistry(t.TempDir(), "A")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := dataset.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	rules := &datasetHTTPRules{rules: []access.Rule{{ID: "allowed", Principal: "alice", Catalog: "A", Actions: []string{"catalog.read", "dataset.manage", "dataset.resolve", "file.read"}}}}
	return &datasetHTTPFixture{catalog: cat, source: source, rules: rules, resources: service.Resources{Catalogs: datasetHTTPCatalogs{cat}, Sources: store, Access: &access.Service{Store: rules}}}
}

func (f *datasetHTTPFixture) handler() *service.Handler {
	return service.NewHandler(service.Config{Identity: service.IdentityOptions{Mode: "local", Local: true},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
			return f.resources, func() {}, nil
		},
	})
}

func datasetHTTPCall(t *testing.T, handler http.Handler, method, target string, body any, want int) map[string]any {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, target, &payload)
	r.Header.Set("X-Kc-As", "alice")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s HTTP %d, want %d: %s", method, target, w.Code, want, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("not JSON: %s", w.Body.String())
	}
	return result
}

func datasetHTTPDefinition(revision int, source kernel.RepositoryID) map[string]any {
	return map[string]any{"dataset": "files", "revision": revision, "sources": []dataset.KnowledgeSetSource{{Repository: source, Selector: snapshot.DefaultRef, File: "plain.txt", Target: "docs/plain.txt"}}}
}

func TestDatasetHTTPStandaloneManagementLifecycle(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	base := "/catalog/v1/catalogs/A/datasets"
	list := datasetHTTPCall(t, h, "GET", "/catalog/v1/catalogs", nil, 200)
	if !reflect.DeepEqual(list["catalogs"], []any{map[string]any{"id": "A"}}) {
		t.Fatalf("Catalog inventory broadened: %#v", list)
	}
	created := datasetHTTPCall(t, h, "POST", base, datasetHTTPDefinition(1, f.source.ID()), 200)
	if created["setId"] != "files" || created["revision"] != float64(1) || len(created) != 3 {
		t.Fatalf("publication wire changed: %#v", created)
	}
	list = datasetHTTPCall(t, h, "GET", base, nil, 200)
	rows := list["datasets"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["itemCount"] != float64(1) {
		t.Fatalf("Dataset inventory lost file selection: %#v", list)
	}
	pin := datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{}, 200)
	if pin["setId"] != "files" || pin["ref"] != "v1" {
		t.Fatalf("pin changed: %#v", pin)
	}
	datasetHTTPCall(t, h, "POST", base+"/files/check", map[string]any{"pin": pin}, 200)
	f.source.head = "c2"
	datasetHTTPCall(t, h, "POST", base, datasetHTTPDefinition(2, f.source.ID()), 200)
	old := datasetHTTPCall(t, h, "GET", base+"/files?revision=1&limit=1", nil, 200)
	if old["revision"] != float64(1) || old["servingRevision"] != float64(2) || old["continuation"] == nil {
		t.Fatalf("history lost its accepted basis: %#v", old)
	}
	if old["sources"].([]any)[0].(map[string]any)["commit"] != "c1" {
		t.Fatalf("historical source followed latest: %#v", old)
	}
	page := datasetHTTPCall(t, h, "GET", base+"/files?continuation="+url.QueryEscape(old["continuation"].(string)), nil, 200)
	if !reflect.DeepEqual(page["versions"], []any{float64(1)}) {
		t.Fatalf("history pagination changed: %#v", page)
	}
	replay := datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{"pin": pin}, 200)
	if replay["repositories"].(map[string]any)[string(f.source.ID())] != "c1" {
		t.Fatalf("old pin drifted: %#v", replay)
	}
	pin["repositories"].(map[string]any)[string(f.source.ID())] = "c2"
	datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{"pin": pin}, 400)
	datasetHTTPCall(t, h, "POST", base+"/files/retire", map[string]any{}, 200)
	datasetHTTPCall(t, h, "POST", base+"/files/resolve", map[string]any{}, 400)
	old = datasetHTTPCall(t, h, "GET", base+"/files?revision=1", nil, 200)
	if old["retired"] != true {
		t.Fatalf("history bypassed current retirement: %#v", old)
	}
}

func TestDatasetHTTPTemporaryLabelCannotBorrowNamedGrant(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	f.rules.rules = []access.Rule{{ID: "named", Principal: "alice", Catalog: "A", Dataset: "files", Actions: []string{"dataset.resolve", "file.read"}}}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 403)
	f.rules.rules = []access.Rule{{ID: "resolve", Principal: "alice", Catalog: "A", Repo: string(f.source.ID()), Actions: []string{"dataset.resolve"}}}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 403)
	f.rules.rules[0].Actions = append(f.rules.rules[0].Actions, "file.read")
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 200)
}

func TestDatasetHTTPTemporarySourceCheckIsCompleteAndPolicyPluggable(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	second := &datasetHTTPSource{id: "kr://files/other", head: "c1"}
	store := f.resources.Sources.(*snapshot.Registry)
	if err := store.Add(second); err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.RegisterRepository(second.ID()); err != nil {
		t.Fatal(err)
	}
	body := datasetHTTPDefinition(1, f.source.ID())
	body["sources"] = []dataset.KnowledgeSetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef, File: "a", Target: "a"}, {Repository: second.ID(), Selector: snapshot.DefaultRef, File: "b", Target: "b"}}
	f.rules.rules = []access.Rule{{ID: "one", Principal: "alice", Catalog: "A", Repo: string(f.source.ID()), Actions: []string{"dataset.resolve", "file.read"}}}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", body, 403)
	f.rules.rules = append(f.rules.rules, access.Rule{ID: "two", Principal: "alice", Catalog: "A", Repo: string(second.ID()), Actions: []string{"dataset.resolve", "file.read"}})
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", body, 200)
	var checked bool
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		if request.Query.Dataset != "" || request.Definition == nil {
			t.Fatalf("temporary label became a grant selector: %#v", request)
		}
		if request.Query.Action == "file.read" {
			checked = true
			if request.Pin == nil || len(request.Pin.Repositories) != 2 {
				t.Fatalf("policy did not get fixed source coordinates: %#v", request)
			}
		}
		return nil
	}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", body, 200)
	if !checked {
		t.Fatal("temporary source consumption bypassed authorization")
	}
}

func TestDatasetHTTPDiscoveryMarkerIsNotAFileGrant(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	if _, err := f.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	var discovered bool
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		discovered = request.CatalogDiscovery
		if !discovered {
			return kernel.Fail(kernel.ErrForbidden, "discovery proof required")
		}
		return nil
	}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets/files/resolve", map[string]any{"catalogDiscovery": true}, 200)
	if !discovered {
		t.Fatal("configured discovery policy marker was lost")
	}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets/files/check", map[string]any{"catalogDiscovery": true}, 400)
	f.resources.Authorize = nil
	f.rules.rules = nil
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets/files/resolve", map[string]any{"catalogDiscovery": true}, 403)
}

func TestDatasetHTTPPublicationPreparesThenSwitchesOnce(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	var candidate dataset.KnowledgeSet
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ *dataset.Catalog, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
		if _, err := f.catalog.Set("files"); err == nil {
			t.Fatal("candidate became current before preparation")
		}
		if definition.Sources[0].Commit != "" {
			t.Fatal("portable recipe got a prematurely frozen source")
		}
		f.source.head = "c2"
		return definition, map[string]any{"recipeFile": ".kc-dataset.yaml", "setId": "hijack"}, nil
	}
	f.resources.PreparePublication = func(_ context.Context, _ identity.HTTPIdentity, definition dataset.KnowledgeSet) error {
		candidate = definition
		if definition.Sources[0].Commit != "c2" {
			t.Fatal("publication missed the prepared source version")
		}
		if _, err := f.catalog.Set("files"); err == nil {
			t.Fatal("current pointer advanced during preparation")
		}
		return nil
	}
	out := datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	if out["setId"] != "files" || out["recipeFile"] != ".kc-dataset.yaml" || len(candidate.Items) != 1 {
		t.Fatalf("publication receipt or scope changed: %#v", out)
	}
	accepted, err := f.catalog.DatasetVersion("files", 1)
	if err != nil || !reflect.DeepEqual(accepted, candidate) {
		t.Fatalf("prepared version differs from accepted record: %#v,%v", accepted, err)
	}
}

func TestDatasetHTTPPreparationFailureKeepsServingVersion(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	f.resources.PreparePublication = func(context.Context, identity.HTTPIdentity, dataset.KnowledgeSet) error {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "capability not ready")
	}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(2, f.source.ID()), 503)
	current, err := f.catalog.Set("files")
	if err != nil || current.Revision != 1 {
		t.Fatalf("failed preparation switched publication: %#v,%v", current, err)
	}
	if _, err := f.catalog.DatasetVersion("files", 2); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unaccepted candidate became history: %v", err)
	}
}

func TestDatasetHTTPRejectsInvalidWireAndHistoryQueries(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	base := "/catalog/v1/catalogs/A/datasets"
	for _, body := range []any{map[string]any{"dataset": "files", "revision": 1, "sources": []any{map[string]any{"objectId": "Entity:a"}}}, map[string]any{"dataset": "files", "revision": 1, "sources": []any{}}, map[string]any{"dataset": "files", "revision": 1, "sources": []any{}, "extra": true}} {
		datasetHTTPCall(t, f.handler(), "POST", base, body, 400)
	}
	datasetHTTPCall(t, f.handler(), "POST", base, datasetHTTPDefinition(1, f.source.ID()), 200)
	for _, query := range []string{"revision=0", "revision=-1", "revision=nope", "limit=201", "limit=-1", "limit=nope", "continuation=invalid"} {
		datasetHTTPCall(t, f.handler(), "GET", base+"/files?"+query, nil, 400)
	}
	datasetHTTPCall(t, f.handler(), "GET", base+"/files?revision=2", nil, 404)
}

func TestDatasetHTTPNamedResolveDoesNotGrantTemporarySources(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	if _, err := f.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	f.rules.rules = []access.Rule{{ID: "named", Principal: "alice", Catalog: "A", Dataset: "files", Actions: []string{"dataset.resolve"}}}
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets/files/resolve", map[string]any{}, 200)
	datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 403)
}

func TestDatasetHTTPBeforePublishCannotRedirectOrContinueAfterRevocation(t *testing.T) {
	for _, mode := range []string{"identity", "revoked", "invalid-revision"} {
		t.Run(mode, func(t *testing.T) {
			f := newDatasetHTTPFixture(t)
			called := false
			f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ *dataset.Catalog, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
				called = true
				if mode == "identity" {
					definition.SetID = "other"
				} else {
					f.rules.rules = nil
				}
				return definition, nil, nil
			}
			revision, status := 1, 403
			if mode == "identity" {
				status = 409
			}
			if mode == "invalid-revision" {
				revision, status = 0, 400
			}
			datasetHTTPCall(t, f.handler(), "POST", "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(revision, f.source.ID()), status)
			if mode == "invalid-revision" && called {
				t.Fatal("invalid candidate caused a portable-recipe side effect")
			}
			if state := f.catalog.DumpState(); len(state.KnowledgeSets) != 0 || len(state.DatasetVersions) != 0 {
				t.Fatalf("rejected preparation accepted a release: %#v", state)
			}
		})
	}
}

func TestDatasetHTTPBodyLabelsAreDiagnosticScopeOnly(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	observed := []string{}
	f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		if request.Definition != nil && request.Query.Dataset != "" {
			t.Fatalf("diagnostic recipe label entered authorization: %#v", request)
		}
		return nil
	}
	h := service.NewHandler(service.Config{
		Identity: service.IdentityOptions{Mode: "local", Local: true},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
			return f.resources, func() {}, nil
		},
		Observe: func(ctx context.Context, _ identity.HTTPIdentity, operation string) (context.Context, func(any, error)) {
			metadata := service.ContextMetadata(ctx)
			if metadata.Catalog != "A" || metadata.Dataset != "files" {
				t.Fatalf("body scope missing from diagnostic aspect: %#v", metadata)
			}
			observed = append(observed, operation)
			return ctx, nil
		},
	})
	datasetHTTPCall(t, h, "POST", "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	datasetHTTPCall(t, h, "POST", "/catalog/v1/catalogs/A/datasets:resolve", datasetHTTPDefinition(1, f.source.ID()), 200)
	if !reflect.DeepEqual(observed, []string{"dataset-define", "pin-source"}) {
		t.Fatalf("operation aspects changed: %v", observed)
	}
}

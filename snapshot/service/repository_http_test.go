package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestSnapshotRepositoryHTTPFeedsStandaloneFilePublication(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	store := fixture.resources.Sources.(*snapshot.Registry)
	unregistered := &datasetHTTPSource{id: "kr://another-catalog/source", head: "c1"}
	if err := store.Add(unregistered); err != nil {
		t.Fatal(err)
	}
	handler := fixture.handler()
	inventory := datasetHTTPCall(t, handler, http.MethodGet, "/catalog/v1/catalogs/A/repositories", nil, http.StatusOK)
	if inventory["catalogId"] != "A" || !reflect.DeepEqual(inventory["repositories"], []any{map[string]any{"id": string(source.id)}}) {
		t.Fatalf("inventory exposed sources outside this Catalog: %#v", inventory)
	}
	if source.reads != 0 || source.scans != 0 {
		t.Fatal("source identity inventory read file contents")
	}
	selected := inventory["repositories"].([]any)[0].(map[string]any)["id"].(string)
	body := map[string]any{"dataset": "files", "revision": 1, "sources": []any{map[string]any{
		"repository": selected, "selector": snapshot.DefaultRef,
		"file": "reference/a.json", "target": "shared/data.json",
	}}}
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets:resolve", body, http.StatusOK)
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets", body, http.StatusOK)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets/files/resolve", map[string]any{}, http.StatusOK)
	read := datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", map[string]any{"catalog": "A", "dataset": "files", "pin": pin, "path": "shared/data.json"}, http.StatusOK)
	raw, err := json.Marshal(read)
	if err != nil {
		t.Fatal(err)
	}
	var delivered struct{ Content []byte }
	if err := json.Unmarshal(raw, &delivered); err != nil {
		t.Fatal(err)
	}
	if string(delivered.Content) != `{"object_id":"Business:old","value":1}` {
		t.Fatalf("standalone inventory did not lead to fixed file delivery: %s", raw)
	}
}

func TestSnapshotRepositoryHTTPInventoryDoesNotGrantOtherScopes(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	fixture.rules.rules[0].Actions = []string{"catalog.read", "dataset.resolve"}
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: source.id, Selector: snapshot.DefaultRef, File: "reference/a.json", Target: "public.json"}}); err != nil {
		t.Fatal(err)
	}
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodGet, "/catalog/v1/catalogs/A/repositories", nil, http.StatusOK)
	datasetHTTPCall(t, handler, http.MethodGet, "/catalog/v1/catalogs/B/repositories", nil, http.StatusForbidden)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets/files/resolve", map[string]any{}, http.StatusOK)
	datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", map[string]any{"catalog": "A", "dataset": "files", "pin": pin, "path": "public.json"}, http.StatusForbidden)
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(2, source.id), http.StatusForbidden)
	if source.reads != 0 {
		t.Fatal("Catalog inventory visibility granted source bytes")
	}
	fixture.rules.rules = []access.Rule{{ID: "file-only", Principal: "alice", Repo: string(source.id), Actions: []string{"file.read"}}}
	datasetHTTPCall(t, handler, http.MethodGet, "/catalog/v1/catalogs/A/repositories", nil, http.StatusForbidden)
}

func TestSnapshotRepositoryHTTPOptionalDetailsCannotOverrideIdentity(t *testing.T) {
	fixture := newDatasetHTTPFixture(t)
	called := []kernel.RepositoryID{}
	fixture.resources.DescribeRepository = func(_ context.Context, caller identity.HTTPIdentity, id kernel.RepositoryID) (map[string]any, error) {
		if caller.Principal != "alice" {
			t.Fatalf("inventory extension lost verified caller: %#v", caller)
		}
		called = append(called, id)
		return map[string]any{"id": "kr://unregistered/source", "schemaCount": 3}, nil
	}
	listed := datasetHTTPCall(t, fixture.handler(), http.MethodGet, "/catalog/v1/catalogs/A/repositories", nil, http.StatusOK)
	rows := listed["repositories"].([]any)
	if !reflect.DeepEqual(rows, []any{map[string]any{"id": string(fixture.source.id), "schemaCount": float64(3)}}) || !reflect.DeepEqual(called, []kernel.RepositoryID{fixture.source.id}) {
		t.Fatalf("extension redirected an authoritative source identity: %#v, calls=%v", listed, called)
	}
	datasetHTTPCall(t, fixture.handler(), http.MethodGet, "/catalog/v1/catalogs/B/repositories", nil, http.StatusForbidden)
	if len(called) != 1 {
		t.Fatal("unauthorized Catalog executed the inventory extension")
	}
	fixture.resources.DescribeRepository = func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) (map[string]any, error) {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "external metadata unavailable")
	}
	datasetHTTPCall(t, fixture.handler(), http.MethodGet, "/catalog/v1/catalogs/A/repositories", nil, http.StatusServiceUnavailable)
}

func TestSnapshotRepositoryHTTPRejectsProviderCatalogAlias(t *testing.T) {
	fixture := newDatasetHTTPFixture(t)
	registry, err := dataset.NewRegistry(t.TempDir(), "B")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := dataset.NewCatalog(fixture.resources.Sources.(*snapshot.Registry), registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := foreign.RegisterRepository(fixture.source.id); err != nil {
		t.Fatal(err)
	}
	fixture.resources.Catalogs = datasetHTTPCatalogs{catalog: foreign}
	extended := false
	fixture.resources.DescribeRepository = func(context.Context, identity.HTTPIdentity, kernel.RepositoryID) (map[string]any, error) {
		extended = true
		return nil, nil
	}
	datasetHTTPCall(t, fixture.handler(), http.MethodGet, "/catalog/v1/catalogs/A/repositories", nil, http.StatusConflict)
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/catalog/v1/catalogs/A/datasets", datasetHTTPDefinition(1, fixture.source.id), http.StatusConflict)
	if extended || len(foreign.DumpState().KnowledgeSets) != 0 || len(fixture.catalog.DumpState().KnowledgeSets) != 0 {
		t.Fatal("provider alias leaked inventory or accepted a cross-Catalog publication")
	}
}

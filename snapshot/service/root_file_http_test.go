package service_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func rootFileDefinition(source kernel.RepositoryID, revision int) map[string]any {
	return map[string]any{"dataset": "root", "revision": revision, "sources": []any{map[string]any{
		"repository": source, "selector": snapshot.DefaultRef, "path": "", "subPath": "reference",
	}}}
}

func rootFileRead(t *testing.T, response map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct{ Content []byte }
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return string(decoded.Content)
}

func TestSnapshotRootPrefixReadsDeliveredAndMountRelativeFiles(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets", rootFileDefinition(source.id, 1), http.StatusOK)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets/root/resolve", map[string]any{}, http.StatusOK)
	for _, address := range []map[string]any{{"path": "a.json"}, {"mountPath": "", "file": "a.json"}} {
		request := map[string]any{"catalog": "A", "dataset": "root", "pin": pin}
		for key, value := range address {
			request[key] = value
		}
		read := datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", request, http.StatusOK)
		if rootFileRead(t, read) != `{"object_id":"Business:old","value":1}` || read["mount"].(map[string]any)["commit"] != "c1" {
			t.Fatalf("root prefix did not map into its fixed source subtree: %#v", read)
		}
	}
	page := datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/tree:list", map[string]any{"catalog": "A", "dataset": "root", "pin": pin, "path": ""}, http.StatusOK)
	if len(page["entries"].([]any)) != 2 {
		t.Fatalf("root subtree included an unselected neighbor: %#v", page)
	}
}

func TestSnapshotRootPrefixKeepsHistoricalBasisAndCurrentPermission(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets", rootFileDefinition(source.id, 1), http.StatusOK)
	old := datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets/root/resolve", map[string]any{}, http.StatusOK)
	source.head = "c2"
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets", rootFileDefinition(source.id, 2), http.StatusOK)
	current := datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets/root/resolve", map[string]any{}, http.StatusOK)
	read := func(pin map[string]any, file string, want int) map[string]any {
		return datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", map[string]any{"catalog": "A", "dataset": "root", "pin": pin, "path": file}, want)
	}
	if value := rootFileRead(t, read(old, "a.json", http.StatusOK)); value != `{"object_id":"Business:old","value":1}` {
		t.Fatalf("source advance changed the historical root bytes: %s", value)
	}
	if value := rootFileRead(t, read(current, "renamed.json", http.StatusOK)); value != `{"object_id":"Business:old","value":2}` {
		t.Fatalf("new root release did not select its own commit: %s", value)
	}
	read(current, "a.json", http.StatusNotFound)
	history := datasetHTTPCall(t, handler, http.MethodGet, "/catalog/v1/catalogs/A/datasets/root?revision=1", nil, http.StatusOK)
	if history["sources"].([]any)[0].(map[string]any)["commit"] != "c1" || history["servingRevision"] != float64(2) {
		t.Fatalf("historical root coordinates drifted: %#v", history)
	}
	before := source.reads
	fixture.rules.rules[0].Actions = []string{"catalog.read", "dataset.resolve", "dataset.manage"}
	read(old, "a.json", http.StatusForbidden)
	if source.reads != before {
		t.Fatal("old root pin bypassed current file-read revocation")
	}
}

func TestSnapshotRootPrefixRejectsMixedAddressesForgedScopeAndEscapes(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets", rootFileDefinition(source.id, 1), http.StatusOK)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/catalog/v1/catalogs/A/datasets/root/resolve", map[string]any{}, http.StatusOK)
	before := source.reads
	for _, address := range []map[string]any{
		{"path": "a.json", "file": "a.json"},
		{"path": "a.json", "mountPath": "other"},
		{"file": "../private/hidden.txt"},
		{"file": "scratch/../../private/hidden.txt"},
		{"path": "../private/hidden.txt"},
		{"path": "scratch/../../private/hidden.txt"},
	} {
		request := map[string]any{"catalog": "A", "dataset": "root", "pin": pin}
		for key, value := range address {
			request[key] = value
		}
		datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", request, http.StatusBadRequest)
	}
	if source.reads != before {
		t.Fatal("invalid root addressing reached source bytes")
	}
	raw, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	var forged map[string]any
	if err := json.Unmarshal(raw, &forged); err != nil {
		t.Fatal(err)
	}
	forged["items"].([]any)[0].(map[string]any)["prefix"] = ""
	datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", map[string]any{"catalog": "A", "dataset": "root", "pin": forged, "path": "private/hidden.txt"}, http.StatusBadRequest)
	if source.reads != before {
		t.Fatal("forged root pin broadened the accepted prefix")
	}
}

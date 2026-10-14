package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"kc/identity"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestDatasetHTTPMetadataPublicationAndHistoricalRead(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	want := map[string]string{"description": "plain delivery", "team/custom": "茶铺", "owner": "untrusted attribute", "empty": ""}
	preparations := 0
	f.resources.PreparePublication = func(_ context.Context, _ identity.HTTPIdentity, definition dataset.Dataset) error {
		preparations++
		if definition.Revision == 1 && !reflect.DeepEqual(definition.Metadata, want) {
			t.Fatalf("publication preparation lost metadata: %#v", definition)
		}
		return nil
	}
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.Dataset) (dataset.Dataset, map[string]any, error) {
		if definition.Revision == 1 && !reflect.DeepEqual(definition.Metadata, want) {
			t.Fatalf("portable recipe preparation lost metadata: %#v", definition)
		}
		return definition, nil, nil
	}
	h := f.handler()
	base := "/snapshot/v1/datasets"
	request := datasetHTTPDefinition(1, f.source.ID())
	request["metadata"] = want
	created := datasetHTTPCall(t, h, http.MethodPost, base, request, 200)
	assertDatasetMetadata(t, created, want)
	list := datasetHTTPCall(t, h, http.MethodGet, base, nil, 200)
	assertDatasetMetadata(t, list["datasets"].([]any)[0].(map[string]any), want)
	current := datasetHTTPCall(t, h, http.MethodGet, base+"/files", nil, 200)
	assertDatasetMetadata(t, current, want)
	// Metadata changes do not alter the selected source or delivery target.
	first, err := f.catalog.DatasetVersion("files", 1)
	if err != nil || len(first.Items) != 1 || first.Items[0].Target != "docs/plain.txt" {
		t.Fatalf("resource attributes changed file delivery: %#v %v", first, err)
	}
	request = datasetHTTPDefinition(2, f.source.ID())
	request["metadata"] = map[string]string{"description": "new release"}
	datasetHTTPCall(t, h, http.MethodPost, base, request, 200)
	assertDatasetMetadata(t, datasetHTTPCall(t, h, http.MethodGet, base+"/files?revision=1", nil, 200), want)
	datasetHTTPCall(t, h, http.MethodPost, base, datasetHTTPDefinition(3, f.source.ID()), 200)
	cleared := datasetHTTPCall(t, h, http.MethodGet, base+"/files", nil, 200)
	if cleared["metadata"] != nil || preparations != 3 {
		t.Fatalf("omitted metadata was inherited or release was not prepared: %#v %d", cleared, preparations)
	}
	assertDatasetMetadata(t, datasetHTTPCall(t, h, http.MethodGet, base+"/files?revision=2", nil, 200), map[string]string{"description": "new release"})
}

func TestDatasetHTTPMetadataDoesNotGrantManagementOrFileAccess(t *testing.T) {
	f, source := newFileHTTPFixture(t)
	h := f.handler()
	request := map[string]any{
		"dataset": "files", "revision": 1,
		"sources":  []dataset.DatasetSource{{Repository: source.id, Selector: "refs/heads/main", File: "reference/a.json", Target: "public.json"}},
		"metadata": map[string]string{"owner": "alice", "actions": "*", "path": "private/hidden.txt"},
	}
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", request, http.StatusOK)
	pin := datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	source.reads = 0 // Begin observing consumption after publication validation.
	// Keep resolve authority so a missing file.read cannot be hidden by an
	// earlier permission failure. Neither accepted owner nor submitted actions
	// replace the current management and byte-delivery checks.
	f.rules.rules = []access.Rule{{ID: "resolve-only", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve"}}}
	before := dataset.NormalizeState(f.catalog.DumpState())
	source.inspections = 0
	request["revision"] = 2
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/datasets", request, http.StatusForbidden)
	assertDatasetHTTPState(t, f, before)
	if source.inspections != 0 {
		t.Fatal("submitted resource attributes bypassed management authorization")
	}
	datasetHTTPCall(t, h, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "public.json"}, http.StatusForbidden)
	if source.reads != 0 {
		t.Fatal("accepted resource owner bypassed byte authorization")
	}
	assertDatasetHTTPState(t, f, before)
}

func assertDatasetMetadata(t *testing.T, view map[string]any, want map[string]string) {
	t.Helper()
	values, ok := view["metadata"].(map[string]any)
	if !ok || len(values) != len(want) {
		t.Fatalf("missing or changed metadata: %#v", view)
	}
	for key, value := range want {
		if values[key] != value {
			t.Fatalf("metadata %q = %#v, want %q", key, values[key], value)
		}
	}
}

func TestDatasetHTTPInvalidMetadataCannotPublish(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	h := f.handler()
	for _, metadata := range []string{`null`, `[]`, `{"x":null}`, `{"x":123}`, `{"x":true}`, `{"x":{"nested":"value"}}`, `{"x":"first","x":"second"}`} {
		t.Run(metadata, func(t *testing.T) {
			body := `{"dataset":"invalid","revision":1,"sources":[{"repository":"kr://files/source","selector":"refs/heads/main"}],"metadata":` + metadata + `}`
			r := httptest.NewRequest(http.MethodPost, "/snapshot/v1/datasets", strings.NewReader(body))
			r.Header.Set("X-Kc-As", "alice")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest || len(f.catalog.DumpState().Datasets) != 0 {
				t.Fatalf("invalid metadata accepted or publication changed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

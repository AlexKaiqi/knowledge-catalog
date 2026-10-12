package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func TestSnapshotExternalFileViewMissingResultFailsBeforeDelivery(t *testing.T) {
	for _, route := range []string{"mounts:list", "tree:list", "file:read"} {
		t.Run(route, func(t *testing.T) {
			fixture, source := newFileHTTPFixture(t)
			if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: source.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs"), SubPath: "reference"}}); err != nil {
				t.Fatal(err)
			}
			pin, err := fixture.catalog.ResolveKnowledgeSet("files")
			if err != nil {
				t.Fatal(err)
			}
			called, released := 0, 0
			fixture.resources.OpenView = func(context.Context, identity.HTTPIdentity, service.FileCoordinate, *dataset.Catalog, dataset.KnowledgeSet, dataset.ResolvedKnowledgeSet) (*service.FileView, error) {
				called++
				return nil, nil
			}
			handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
				return fixture.resources, func() { released++ }, nil
			}})
			raw, err := json.Marshal(map[string]any{"catalog": "A", "dataset": "files", "pin": pin, "view": "semantic"})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/dataset-files/v1/"+route, bytes.NewReader(raw))
			request.Header.Set("X-Kc-As", "alice")
			response := httptest.NewRecorder()
			var panicked any
			func() {
				defer func() { panicked = recover() }()
				handler.ServeHTTP(response, request)
			}()
			if panicked != nil {
				t.Fatalf("missing external view crashed HTTP instead of reporting its port failure: %v", panicked)
			}
			var failure struct {
				Error struct{ Code kernel.ErrorCode }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || response.Code != http.StatusConflict || failure.Error.Code != kernel.ErrPreconditionFailed {
				t.Fatalf("missing external view did not fail its port contract: HTTP %d %s, %v", response.Code, response.Body.String(), err)
			}
			if called != 1 || released != 1 || source.reads != 0 {
				t.Fatalf("external-view failure lost lease or read bytes: called=%d released=%d reads=%d", called, released, source.reads)
			}
		})
	}
}

func TestSnapshotExternalFileViewPreservesCallbackFailure(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: source.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs"), SubPath: "reference"}}); err != nil {
		t.Fatal(err)
	}
	pin, err := fixture.catalog.ResolveKnowledgeSet("files")
	if err != nil {
		t.Fatal(err)
	}
	called, released := 0, 0
	fixture.resources.OpenView = func(context.Context, identity.HTTPIdentity, service.FileCoordinate, *dataset.Catalog, dataset.KnowledgeSet, dataset.ResolvedKnowledgeSet) (*service.FileView, error) {
		called++
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "external view unavailable")
	}
	handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
		return fixture.resources, func() { released++ }, nil
	}})
	failure := datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/file:read", map[string]any{"catalog": "A", "dataset": "files", "pin": pin, "view": "semantic"}, http.StatusServiceUnavailable)
	if fault := failure["error"].(map[string]any); fault["code"] != string(kernel.ErrTemporaryUnavailable) || fault["message"] != "external view unavailable" {
		t.Fatalf("external-view error changed: %#v", failure)
	}
	if called != 1 || released != 1 || source.reads != 0 {
		t.Fatalf("external-view failure lost lease or read bytes: called=%d released=%d reads=%d", called, released, source.reads)
	}
}

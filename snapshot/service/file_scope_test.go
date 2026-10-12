package service_test

import (
	"context"
	"net/http"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

type auditedDatasetService struct {
	dataset.Service
	opened int
}

func (s *auditedDatasetService) Set(id string) (dataset.KnowledgeSet, error) {
	s.opened++
	return s.Service.Set(id)
}

func TestSnapshotFileAuthorizationPrecedesDatasetLookup(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	fixture.rules.rules = nil
	authority := &auditedDatasetService{Service: fixture.resources.Datasets}
	fixture.resources.Datasets = authority
	for _, id := range []string{"files", "unknown"} {
		response := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": id}, http.StatusForbidden)
		if response["error"].(map[string]any)["code"] != string(kernel.ErrForbidden) {
			t.Fatalf("Dataset existence affected unauthorized response: %#v", response)
		}
	}
	if authority.opened != 0 || source.reads != 0 {
		t.Fatalf("unauthorized request reached Dataset/source lookup: opened=%d reads=%d", authority.opened, source.reads)
	}
}

func TestSnapshotFileMissingDatasetAuthorityFailsClosed(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	fixture.resources.Datasets = nil
	response := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": "files"}, http.StatusConflict)
	if response["error"].(map[string]any)["code"] != string(kernel.ErrPreconditionFailed) || source.reads != 0 {
		t.Fatalf("missing authority did not fail before delivery: %#v reads=%d", response, source.reads)
	}
}

func TestSnapshotFileRejectsRetiredCatalogSelectorBeforeLookup(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	authority := &auditedDatasetService{Service: fixture.resources.Datasets}
	fixture.resources.Datasets = authority
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": "files", "catalog": "foreign"}, http.StatusBadRequest)
	if authority.opened != 0 || source.reads != 0 {
		t.Fatal("retired selector reached authority or source")
	}
}

func TestSnapshotFileRechecksCurrentAuthorizationBeforeBytes(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: source.id, Selector: "refs/heads/main", File: "reference/a.json", Target: "public.json"}}); err != nil {
		t.Fatal(err)
	}
	checks := []string{}
	fixture.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		checks = append(checks, request.Query.Action)
		if request.Query.Action == "file.read" {
			return kernel.Fail(kernel.ErrForbidden, "current file access denied")
		}
		query := request.Query
		query.Principal = request.Caller.Principal
		if _, ok := access.Match(fixture.rules.rules, query); !ok {
			return kernel.Fail(kernel.ErrForbidden, "not allowed")
		}
		return nil
	}
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": "files"}, http.StatusOK)
	if len(checks) < 2 || checks[0] != "dataset.resolve" || checks[len(checks)-1] != "file.read" || source.reads != 0 {
		t.Fatalf("delivery did not apply current file access: checks=%v reads=%d", checks, source.reads)
	}
}

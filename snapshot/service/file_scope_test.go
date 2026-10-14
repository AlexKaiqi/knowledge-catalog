package service_test

import (
	"context"
	"net/http"
	"reflect"
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
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: source.id, Selector: "refs/heads/main", File: "reference/a.json", Target: "public.json"}}); err != nil {
		t.Fatal(err)
	}
	source.reads = 0 // Publication is outside the unauthorized lookup observation.
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
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{
		{Repository: source.id, Selector: "refs/heads/main", Path: dataset.MountPath("docs"), SubPath: "reference"},
		{Repository: source.id, Selector: "refs/heads/main", File: "reference/a.json", Target: "public.json"},
	}); err != nil {
		t.Fatal(err)
	}
	pin, err := fixture.catalog.ResolveKnowledgeSet("files")
	if err != nil {
		t.Fatal(err)
	}
	source.reads = 0 // Observe current consumer authorization after publication.
	checks := []string{}
	fixture.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		want := access.Query{Principal: "alice", Action: request.Query.Action, Dataset: "files"}
		if request.Query.Action == "file.read" {
			want.Repo = string(source.id)
		}
		if request.Caller.Principal != "alice" || !reflect.DeepEqual(request.Query, want) || request.NativeRepository || request.InventoryDiscovery || request.Definition != nil {
			t.Fatalf("file authorization lost its verified Dataset caller: %#v", request)
		}
		checks = append(checks, request.Query.Action)
		if request.Query.Action == "file.read" {
			if request.Query.Repo != string(source.id) || request.Pin == nil || !reflect.DeepEqual(*request.Pin, pin) {
				t.Fatalf("file authorization lost its accepted source basis: %#v", request)
			}
			return kernel.Fail(kernel.ErrForbidden, "current file access denied")
		}
		if request.Query.Action != "dataset.resolve" || request.Query.Repo != "" || request.Pin != nil {
			t.Fatalf("unexpected Dataset authorization coordinate: %#v", request)
		}
		if _, ok := access.Match(fixture.rules.rules, request.Query); !ok {
			return kernel.Fail(kernel.ErrForbidden, "not allowed")
		}
		return nil
	}
	mounts := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": "files"}, http.StatusOK)
	if len(mounts["mounts"].([]any)) != 0 {
		t.Fatalf("denied source was exposed as a readable mount: %#v", mounts)
	}
	// The pin remains the accepted publication basis. Current permission must
	// instead restrict both directory mounts and individual delivered items.
	page := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": mounts["pin"], "path": ""}, http.StatusOK)
	if len(page["entries"].([]any)) != 0 || page["exhausted"] != true {
		t.Fatalf("denied source contributed delivered children: %#v", page)
	}
	for _, path := range []string{"docs/a.json", "public.json"} {
		failed := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": mounts["pin"], "path": path}, http.StatusForbidden)
		if failed["error"].(map[string]any)["code"] != string(kernel.ErrForbidden) {
			t.Fatalf("denied source did not retain its authorization failure: %#v", failed)
		}
	}
	if len(checks) < 2 || checks[0] != "dataset.resolve" || checks[len(checks)-1] != "file.read" || source.reads != 0 {
		t.Fatalf("delivery did not apply current file access: checks=%v reads=%d", checks, source.reads)
	}
}

package service_test

import (
	"net/http"
	"testing"

	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestSnapshotFileMetadataReplayNeverReadsBodiesBeforeCurrentAuthorization(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	if _, err := fixture.catalog.DefineDataset("files", 1, []dataset.DatasetSource{
		{Repository: source.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs"), SubPath: "reference"},
		{Repository: source.id, Selector: snapshot.DefaultRef, File: "reference/a.json", Target: "outside.json"},
	}); err != nil {
		t.Fatal(err)
	}
	source.reads, source.scans = 0, 0
	handler := fixture.handler()
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	for _, route := range []string{"mounts:list", "tree:list"} {
		request := map[string]any{"dataset": "files", "pin": pin}
		if route == "tree:list" {
			request["path"] = ""
		}
		datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/"+route, request, http.StatusOK)
	}
	if source.reads != 0 || source.scans != 0 {
		t.Fatalf("resolve/mounts/list replay probed bodies or whole tree: reads=%d scans=%d", source.reads, source.scans)
	}
	fixture.rules.rules = []access.Rule{{ID: "resolve-only", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve"}}}
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "outside.json"}, http.StatusForbidden)
	if source.reads != 0 {
		t.Fatal("fixed pin replay read a body before file.read permission")
	}
}

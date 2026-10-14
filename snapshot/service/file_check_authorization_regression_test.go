package service_test

import (
	"net/http"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestSnapshotDatasetCheckAuthorizesAllFixedSourcesBeforeBodyProbes(t *testing.T) {
	fixture, first := newFileHTTPFixture(t)
	second := &fileHTTPSource{datasetHTTPSource: &datasetHTTPSource{id: "kr://files/second", head: "c1"}, files: map[kernel.CommitID]map[string][]byte{"c1": {"selected.txt": []byte("second body")}}}
	if err := fixture.resources.Sources.(snapshot.Registry).Add(second); err != nil {
		t.Fatal(err)
	}
	if err := fixture.catalog.RegisterRepository(second.id); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{
		{Repository: first.id, Selector: snapshot.DefaultRef, File: "reference/a.json", Target: "first.json"},
		{Repository: second.id, Selector: snapshot.DefaultRef, File: "selected.txt", Target: "second.txt"},
	}); err != nil {
		t.Fatal(err)
	}
	first.reads, second.reads = 0, 0
	head := testkit.MustHead(t, fixture.authority, snapshot.DefaultRef)
	resolve := access.Rule{ID: "resolve", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve"}}
	fileRead := access.Rule{ID: "read-first", Principal: "alice", Dataset: "files", Repo: string(first.id), Ref: "c1", Actions: []string{"file.read"}}
	for _, grants := range [][]access.Rule{{resolve}, {resolve, fileRead}} {
		fixture.rules.rules = grants
		datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets/files/check", map[string]any{}, http.StatusForbidden)
		if first.reads != 0 || second.reads != 0 || testkit.MustHead(t, fixture.authority, snapshot.DefaultRef) != head {
			t.Fatal("incomplete file permission probed a body or changed registry HEAD")
		}
	}
	fixture.rules.rules = []access.Rule{resolve, {ID: "read-all", Principal: "alice", Dataset: "files", Ref: "c1", Actions: []string{"file.read"}}}
	// Fault injection represents unavailable content at a retained coordinate.
	delete(second.files["c1"], "selected.txt")
	result := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets/files/check", map[string]any{}, http.StatusOK)
	issues := result["issues"].([]any)
	if result["outcome"] != "FAILED" || len(issues) != 1 || issues[0].(map[string]any)["code"] != string(kernel.ErrKnowledgeRefUnresolved) || first.reads != 1 || second.reads != 1 {
		t.Fatalf("authorized consistency check did not probe actual fixed files: %#v reads=%d/%d", result, first.reads, second.reads)
	}
	if testkit.MustHead(t, fixture.authority, snapshot.DefaultRef) != head {
		t.Fatal("read-only availability check changed registry HEAD")
	}
}

func TestSnapshotDatasetCheckCannotBorrowNamedGrantForForgedFileCoordinates(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: source.id, Selector: snapshot.DefaultRef, File: "reference/a.json", Target: "public.json"}}); err != nil {
		t.Fatal(err)
	}
	pin, err := fixture.catalog.ResolveKnowledgeSet("files")
	if err != nil {
		t.Fatal(err)
	}
	pin.Items[0].File = "private/hidden.txt"
	source.reads = 0
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets/files/check", map[string]any{"pin": pin}, http.StatusBadRequest)
	if source.reads != 0 {
		t.Fatal("client-provided check file list reached source bodies")
	}
}

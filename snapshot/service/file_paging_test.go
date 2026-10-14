package service_test

import (
	"net/http"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func publishPagedFiles(t *testing.T, fixture *datasetHTTPFixture, source *fileHTTPSource, id string, revision int, mount bool, targets ...string) map[string]any {
	t.Helper()
	sources := []dataset.DatasetSource{}
	if mount {
		sources = append(sources, dataset.DatasetSource{Repository: source.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs"), SubPath: "reference"})
	}
	for _, target := range targets {
		sources = append(sources, dataset.DatasetSource{Repository: source.id, Selector: snapshot.DefaultRef, File: "reference/a.json", Target: target})
	}
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets", map[string]any{"dataset": id, "revision": revision, "sources": sources}, http.StatusOK)
	// Publication validates selected files; the following counters observe only
	// consumer paging and its cursor/authorization failure boundaries.
	source.reads, source.scans = 0, 0
	return datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets/"+id+"/resolve", map[string]any{}, http.StatusOK)
}

func collectDeliveredPages(t *testing.T, fixture *datasetHTTPFixture, id, directory string, pin map[string]any, limit int) []string {
	t.Helper()
	names := []string{}
	continuation := ""
	for attempt := 0; attempt < 30; attempt++ {
		page := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": id, "pin": pin, "path": directory, "limit": limit, "continuation": continuation}, http.StatusOK)
		entries := page["entries"].([]any)
		if len(entries) > limit {
			t.Fatalf("delivered page exceeded limit %d: %#v", limit, page)
		}
		for _, entry := range entries {
			names = append(names, entry.(map[string]any)["name"].(string))
		}
		if page["exhausted"] == true {
			return names
		}
		next, _ := page["continuation"].(string)
		if next == "" || next == continuation {
			t.Fatalf("unfinished directory did not advance: %#v", page)
		}
		continuation = next
	}
	t.Fatal("delivered paging did not terminate")
	return nil
}

func TestSnapshotDeliveredDirectoryPagesDerivedAndMixedEntries(t *testing.T) {
	for _, mount := range []bool{false, true} {
		t.Run(map[bool]string{false: "derived", true: "mixed"}[mount], func(t *testing.T) {
			fixture, source := newFileHTTPFixture(t)
			source.files["c1"]["reference/nested/old.txt"] = []byte("old nested bytes")
			pin := publishPagedFiles(t, fixture, source, "files", 1, mount, "docs/c.txt", "docs/d.txt", "docs/nested/new.txt")
			got := collectDeliveredPages(t, fixture, "files", "docs", pin, 1)
			want := []string{"c.txt", "d.txt", "nested"}
			if mount {
				want = append(want, "a.json", "b.txt")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("directory duplicated or lost a child: got %v want %v", got, want)
			}
			if source.reads != 0 || source.scans != 0 {
				t.Fatalf("paging read source bytes or scanned a whole tree: reads=%d scans=%d", source.reads, source.scans)
			}
		})
	}
}

func TestSnapshotDeliveredCursorRejectsDifferentBasisAndVisibility(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	pin := publishPagedFiles(t, fixture, source, "files", 1, true, "docs/c.txt", "docs/d.txt")
	first := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1}, http.StatusOK)
	cursor, _ := first["continuation"].(string)
	if cursor == "" {
		t.Fatal("first bounded mixed page has no continuation")
	}
	other := publishPagedFiles(t, fixture, source, "other", 1, true, "docs/c.txt", "docs/d.txt")
	// Component tables own the complete cursor-coordinate matrix. This HTTP
	// representative proves Dataset, accepted pin and continuation reach it.
	failed := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "other", "pin": other, "path": "docs", "limit": 1, "continuation": cursor}, http.StatusBadRequest)
	if failed["error"].(map[string]any)["code"] != string(kernel.ErrUsageInvalid) {
		t.Fatalf("different Dataset did not fail its cursor contract: %#v", failed)
	}
	fixture.rules.rules = []access.Rule{{ID: "resolve", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve"}}}
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	failed = datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1, "continuation": cursor}, http.StatusConflict)
	if failed["error"].(map[string]any)["code"] != string(kernel.ErrPreconditionFailed) {
		t.Fatalf("changed visible scope did not invalidate its cursor: %#v", failed)
	}
	if source.reads != 0 || source.scans != 0 {
		t.Fatalf("rejected continuation reached source content: reads=%d scans=%d", source.reads, source.scans)
	}
}

func TestSnapshotDeliveredPagingRejectsLateSourceConflict(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	pin := publishPagedFiles(t, fixture, source, "files", 1, true, "docs/z.txt")
	// Inject an invalid old fixed layout after acceptance: current publication
	// rejects this overlap, while consumption must still fail closed for old pins.
	source.files["c1"]["reference/z.txt"] = []byte("conflicting source")
	first := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1}, http.StatusOK)
	if first["exhausted"] == true {
		t.Fatal("partial page claimed to have exhausted the conflicting directory")
	}
	datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "docs/z.txt"}, http.StatusBadRequest)
	cursor := first["continuation"]
	for _, name := range []string{"a.json", "b.txt"} {
		page := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1, "continuation": cursor}, http.StatusOK)
		if page["entries"].([]any)[0].(map[string]any)["name"] != name {
			t.Fatalf("unexpected source page: %#v", page)
		}
		cursor = page["continuation"]
	}
	failed := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1, "continuation": cursor}, http.StatusBadRequest)
	if failed["error"].(map[string]any)["code"] != string(kernel.ErrDatasetInvalid) {
		t.Fatalf("late conflict was treated as a second child: %#v", failed)
	}
}

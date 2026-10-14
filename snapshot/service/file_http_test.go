package service_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
)

// The source is literal bytes at immutable coordinates. It deliberately has
// no object API; even a body containing object_id is just a file.
type fileHTTPSource struct {
	*datasetHTTPSource
	files      map[kernel.CommitID]map[string][]byte
	generation string
	reads      int
	scans      int
}

func (s *fileHTTPSource) ReadFile(file string, commit kernel.CommitID) ([]byte, error) {
	s.reads++
	body, ok := s.files[commit][file]
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "file %s missing", file)
	}
	return append([]byte{}, body...), nil
}
func (s *fileHTTPSource) ListFiles(kernel.CommitID) ([]string, error) {
	s.scans++
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "whole-tree scans are not allowed")
}
func (s *fileHTTPSource) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	prefix := strings.Trim(request.Directory, "/")
	if prefix != "" {
		prefix += "/"
	}
	names := map[string]string{}
	for file := range s.files[request.Commit] {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		relative := strings.TrimPrefix(file, prefix)
		name, _, nested := strings.Cut(relative, "/")
		kind := "file"
		if nested {
			kind = "directory"
		}
		names[name] = kind
	}
	ordered := []string{}
	for name := range names {
		if name > request.Continuation {
			ordered = append(ordered, name)
		}
	}
	sort.Strings(ordered)
	limit := request.Limit
	if limit == 0 {
		limit = 500
	}
	end := min(limit, len(ordered))
	entries := []snapshot.DirectoryEntry{}
	for _, name := range ordered[:end] {
		entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: names[name]})
	}
	continuation := ""
	if end < len(ordered) && end > 0 {
		continuation = ordered[end-1]
	}
	generation := string(request.Commit)
	if s.generation != "" {
		generation = s.generation
	}
	return snapshot.DirectoryPage{Entries: entries, Continuation: continuation, Exhausted: end == len(ordered), Generation: generation}, nil
}
func newFileHTTPFixture(t *testing.T) (*datasetHTTPFixture, *fileHTTPSource) {
	t.Helper()
	fixture := newDatasetHTTPFixture(t)
	source := &fileHTTPSource{datasetHTTPSource: fixture.source, files: map[kernel.CommitID]map[string][]byte{
		"c1": {"reference/a.json": []byte(`{"object_id":"Business:old","value":1}`), "reference/b.txt": []byte("old bytes"), "private/hidden.txt": []byte("must stay outside")},
		"c2": {"reference/renamed.json": []byte(`{"object_id":"Business:old","value":2}`)},
	}}
	registry := fixture.resources.Sources.(snapshot.Registry)
	registry.Delete(source.ID())
	if err := registry.Add(source); err != nil {
		t.Fatal(err)
	}
	return fixture, source
}
func TestSnapshotFileHTTPUsesLiteralCoordinateAndFixedPublishedCommit(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets", map[string]any{
		"dataset": "files", "revision": 1, "sources": []any{map[string]any{"repository": source.id, "selector": snapshot.DefaultRef, "file": "reference/a.json", "target": "shared/first.json"}},
	}, http.StatusOK)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	source.head = "c2"
	response := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "shared/first.json"}, http.StatusOK)
	raw, _ := json.Marshal(response)
	var result struct {
		Content []byte
		Item    struct{ File, Commit string }
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if string(result.Content) != `{"object_id":"Business:old","value":1}` || result.Item.File != "reference/a.json" || result.Item.Commit != "c1" {
		t.Fatalf("file identity drifted: %s", raw)
	}
	before := source.reads
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "Business:old"}, http.StatusForbidden)
	if source.reads != before {
		t.Fatal("a body object_id was used to locate a file")
	}
}
func TestSnapshotFileHTTPPagesOnlySelectedSubtreeAndRejectsGenerationDrift(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets", map[string]any{"dataset": "files", "revision": 1, "sources": []any{map[string]any{"repository": source.id, "selector": snapshot.DefaultRef, "subPath": "reference", "path": "docs"}}}, http.StatusOK)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	page := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1}, http.StatusOK)
	entries := page["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["name"] != "a.json" {
		t.Fatalf("unexpected selected subtree %v", page)
	}
	next := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1, "continuation": page["continuation"]}, http.StatusOK)
	if next["entries"].([]any)[0].(map[string]any)["name"] != "b.txt" || next["exhausted"] != true {
		t.Fatalf("paging lost basis: %v", next)
	}
	source.generation = "c2"
	failed := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/tree:list", map[string]any{"dataset": "files", "pin": pin, "path": "docs", "limit": 1}, http.StatusConflict)
	if failed["error"].(map[string]any)["code"] != string(kernel.ErrPreconditionFailed) {
		t.Fatalf("generation drift was not rejected: %v", failed)
	}
	if source.scans != 0 {
		t.Fatal("directory paging used a whole-tree scan")
	}
}
func TestSnapshotFileHTTPRechecksRevocationAndRejectsForgedFileList(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	handler := fixture.handler()
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets", map[string]any{"dataset": "files", "revision": 1, "sources": []any{map[string]any{"repository": source.id, "selector": snapshot.DefaultRef, "file": "reference/a.json", "target": "public.json"}}}, http.StatusOK)
	pin := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	read := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "public.json"}, http.StatusOK)
	if rootFileRead(t, read) != `{"object_id":"Business:old","value":1}` {
		t.Fatalf("valid accepted file did not remain readable: %#v", read)
	}
	forged := map[string]any{}
	for k, v := range pin {
		forged[k] = v
	}
	forged["items"] = []any{map[string]any{"target": "public.json", "repository": source.id, "commit": "c1", "kind": "file", "file": "private/hidden.txt"}}
	before := source.reads
	failed := datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": forged, "path": "public.json"}, http.StatusBadRequest)
	if failed["error"].(map[string]any)["code"] != string(kernel.ErrDatasetInvalid) || source.reads != before {
		t.Fatalf("forged file list reached delivery or failed for the wrong reason: %#v reads=%d", failed, source.reads)
	}
	fixture.rules.rules = []access.Rule{{ID: "resolve", Principal: "alice", Dataset: "files", Actions: []string{"dataset.resolve"}}}
	datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/datasets/files/resolve", map[string]any{}, http.StatusOK)
	failed = datasetHTTPCall(t, handler, http.MethodPost, "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "public.json"}, http.StatusForbidden)
	if failed["error"].(map[string]any)["code"] != string(kernel.ErrForbidden) {
		t.Fatalf("file-only revocation failed for the wrong reason: %#v", failed)
	}
	if source.reads != before {
		t.Fatal("untrusted pin or revoked permission reached source bytes")
	}
}
func TestSnapshotFileHTTPDoesNotInterpretObjectIdentifiersOrRequireExternalViews(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	response := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/snapshot/v1/datasets", map[string]any{"dataset": "bad", "revision": 1, "sources": []any{map[string]any{"repository": source.id, "selector": snapshot.DefaultRef, "object_id": "Business:old"}}}, http.StatusBadRequest)
	if response["error"].(map[string]any)["code"] != string(kernel.ErrUsageInvalid) {
		t.Fatalf("object identity entered file declaration: %v", response)
	}
	if _, err := fixture.catalog.Set("bad"); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatal("invalid definition created a release")
	}
	if fixture.resources.OpenView != nil || fixture.resources.PreparePublication != nil {
		t.Fatal("standalone file fixture unexpectedly needs external semantics")
	}
}

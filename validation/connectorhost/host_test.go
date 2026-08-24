package connectorhost

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kc/connector"
	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

type fakeKC struct {
	mu         sync.Mutex
	head       string
	commits    []repository.CommitChangeSet
	principals []string
	failCommit bool
}

func (f *fakeKC) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/v1/status":
		_ = json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{{"id": "kr://demo/public/facts", "head": f.head}}})
	case "/v1/commit":
		f.principals = append(f.principals, r.Header.Get("X-Kc-As"))
		if f.failCommit {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "NON_FAST_FORWARD", "message": "simulated stale base"}})
			return
		}
		var request struct {
			CommandID string                     `json:"command-id"`
			ChangeSet repository.CommitChangeSet `json:"changeset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			testHTTPError(w, err)
			return
		}
		old := f.head
		f.head = f.head + "x"
		f.commits = append(f.commits, request.ChangeSet)
		_ = json.NewEncoder(w).Encode(writer.CommitReceipt{CommandID: request.CommandID, Surface: "COMMIT", Disposition: writer.DispositionApplied, Result: writer.CommitResult{RepositoryID: request.ChangeSet.TargetRepository, OldCommit: kernel.CommitID(old), NewCommit: kernel.CommitID(f.head), CommitID: kernel.CommitID(f.head), TargetRef: request.ChangeSet.TargetRef}})
	default:
		http.NotFound(w, r)
	}
}

func testHTTPError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, `{"error":{"message":`+strconvQuote(err.Error())+`}}`)
}

func strconvQuote(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
}

func TestHostLifecyclePreviewCommitReconcileFailureAndGeneration(t *testing.T) {
	repo := copyTestRepo(t)
	fake := &fakeKC{head: strings.Repeat("a", 40)}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer server.Close()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := HostConfig{RepoPath: repo, KCURL: server.URL}
	if err := store.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	host := NewHost(store, config, KCClient{BaseURL: server.URL})

	items, err := host.Connectors(context.Background(), true)
	if err != nil || len(items) != 1 || items[0].Manifest.Metadata.ID != "file-observer" {
		t.Fatalf("discover and validate: %#v %v", items, err)
	}
	preview, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "manual"}, true, false)
	if err != nil || preview.Outcome != RunPreviewed || preview.Summary.Added != 2 {
		t.Fatalf("preview: %#v %v", preview, err)
	}
	state, _ := store.LoadState("file-observer")
	if state.CheckpointVersion != 0 || len(fake.commits) != 0 {
		t.Fatalf("preview advanced state: %#v commits=%d", state, len(fake.commits))
	}

	first, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "manual"}, false, false)
	if err != nil || first.Outcome != RunSucceeded || first.Summary.Added != 2 || first.TargetCommit == "" {
		t.Fatalf("first commit: %#v %v", first, err)
	}
	state, _ = store.LoadState("file-observer")
	if state.CheckpointVersion != 1 || len(fake.commits) != 1 {
		t.Fatalf("first checkpoint: %#v commits=%d", state, len(fake.commits))
	}
	if got := fake.principals[len(fake.principals)-1]; got != "connector/file-observer" {
		t.Fatalf("shared host used wrong connector principal %q", got)
	}

	writeSource(t, repo, "2026-08-24T09:00:00Z", []map[string]string{{"key": "alpha", "value": "ONE"}, {"key": "gamma", "value": "three"}})
	second, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "manual"}, false, false)
	if err != nil || second.Summary.Added != 1 || second.Summary.Updated != 1 || second.Summary.Removed != 1 {
		t.Fatalf("reconcile: %#v %v", second, err)
	}
	state, _ = store.LoadState("file-observer")
	if state.CheckpointVersion != 2 || len(fake.commits) != 2 {
		t.Fatalf("second checkpoint: %#v commits=%d", state, len(fake.commits))
	}
	lastCommit := state.LastCommit
	empty, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "manual"}, false, false)
	if err != nil || empty.Outcome != RunEmpty || empty.CheckpointVersion != 3 {
		t.Fatalf("empty run: %#v %v", empty, err)
	}
	state, _ = store.LoadState("file-observer")
	if state.CheckpointVersion != 3 || state.LastCommit != lastCommit || len(fake.commits) != 2 {
		t.Fatalf("empty run lost commit or wrote changes: %#v commits=%d", state, len(fake.commits))
	}

	writeSource(t, repo, "2026-08-24T10:00:00Z", []map[string]string{{"key": "alpha", "value": "BROKEN"}})
	fake.mu.Lock()
	fake.failCommit = true
	fake.mu.Unlock()
	failed, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "manual"}, false, false)
	if err == nil || failed.Outcome != RunFailed {
		t.Fatalf("expected failed write: %#v %v", failed, err)
	}
	state, _ = store.LoadState("file-observer")
	if state.CheckpointVersion != 3 {
		t.Fatalf("failed write advanced checkpoint: %#v", state)
	}
	fake.mu.Lock()
	fake.failCommit = false
	fake.mu.Unlock()
	retry, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "manual"}, false, false)
	if err != nil || retry.Outcome != RunSucceeded {
		t.Fatalf("retry: %#v %v", retry, err)
	}

	activated, err := host.Activate(context.Background(), "file-observer")
	if err != nil || !activated.Active || activated.ActiveGeneration == "" {
		t.Fatalf("activate: %#v %v", activated, err)
	}
	mainPath := filepath.Join(repo, "connectors", "file-observer", "main.go")
	f, err := os.OpenFile(mainPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\n// generation change\n")
	_ = f.Close()
	if _, err := host.Run(context.Background(), "file-observer", RunTrigger{Kind: "schedule"}, false, true); err == nil {
		t.Fatal("scheduled run must reject changed generation")
	}
}

func TestReconcileRequiresFullCoverage(t *testing.T) {
	m := Manifest{Spec: Spec{Target: Target{Scope: Scope{Aspects: []string{"observed"}}}}}
	err := validateOutput(m, ConnectorOutput{Mode: connector.ModeReconcile, Observation: Observation{SourceRefs: []string{"x"}, ObservedAt: "2026-08-24T00:00:00Z", Representation: "STATE", Coverage: Coverage{Kind: "KEYED"}}})
	if err == nil || !strings.Contains(err.Error(), "FULL") {
		t.Fatalf("expected FULL coverage error, got %v", err)
	}
}

func TestHTTPDashboardAndAPI(t *testing.T) {
	repo := copyTestRepo(t)
	store, _ := NewStore(t.TempDir())
	config := HostConfig{RepoPath: repo, KCURL: "http://kc.invalid"}
	_ = store.SaveConfig(config)
	host := NewHost(store, config, KCClient{BaseURL: config.KCURL})
	server := httptest.NewServer(host.HTTPHandler())
	defer server.Close()
	res, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "file-observer") {
		t.Fatalf("dashboard: %s %s", res.Status, body)
	}
	res, err = http.Get(server.URL + "/api/connectors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
}

func copyTestRepo(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("testdata", "repo"))
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "repo")
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, path)
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

func writeSource(t *testing.T, repo, capturedAt string, facts []map[string]string) {
	t.Helper()
	body, _ := json.MarshalIndent(map[string]any{"sourceRef": "file://example/facts.json", "capturedAt": capturedAt, "facts": facts}, "", "  ")
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(repo, "sources", "facts.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerTickStartsDueActiveConnector(t *testing.T) {
	repo := copyTestRepo(t)
	fake := &fakeKC{head: strings.Repeat("b", 40)}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer server.Close()
	store, _ := NewStore(t.TempDir())
	config := HostConfig{RepoPath: repo, KCURL: server.URL}
	_ = store.SaveConfig(config)
	host := NewHost(store, config, KCClient{BaseURL: server.URL})
	state, err := host.Activate(context.Background(), "file-observer")
	if err != nil {
		t.Fatal(err)
	}
	state.NextRunAt = nowString(time.Now().Add(-time.Second))
	if err := store.SaveState(state); err != nil {
		t.Fatal(err)
	}
	host.Tick(context.Background())
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := store.Runs("file-observer", 10)
		if len(runs) > 0 {
			if runs[0].Outcome != RunSucceeded {
				t.Fatalf("scheduled run: %#v", runs[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduled run did not finish")
}

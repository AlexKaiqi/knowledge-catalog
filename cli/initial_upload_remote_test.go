package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	kcclient "kc/client"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

const initialUploadRepository = "kr://acme/initial-upload"

type initialUploadRemote struct {
	mu             sync.Mutex
	head           string
	headStatus     int
	commitError    kernel.ErrorCode
	receiptError   kernel.ErrorCode
	resolveErrorAt string
	advance        bool
	current        map[string]kernel.Digest
	history        map[string]map[string]kernel.Digest
	heads          []string
	resolves       []kcclient.ObjectResolveRequest
	commits        []kcclient.CommitRequest
	receipt        objectmodel.CommitReceipt
	entry          snapshot.CommandEntry
	receipts       int
}

func newInitialUploadRemote(t *testing.T, head string) (*initialUploadRemote, *httptest.Server) {
	t.Helper()
	isolateLoginConfig(t)
	f := &initialUploadRemote{head: head, current: map[string]kernel.Digest{}, history: map[string]map[string]kernel.Digest{}}
	server := httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(server.Close)
	return f, server
}

func (f *initialUploadRemote) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/head"):
		f.heads = append(f.heads, r.URL.Query().Get("ref"))
		if f.headStatus != 0 {
			w.WriteHeader(f.headStatus)
		}
		_, _ = w.Write([]byte(f.head))
	case r.Method == http.MethodPost && r.URL.Path == "/object/v1/objects:resolve":
		var request kcclient.ObjectResolveRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.resolves = append(f.resolves, request)
		if request.Commit == f.resolveErrorAt {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(kernel.ErrForbidden, "original basis denied")))
			return
		}
		current := f.current
		if original, ok := f.history[request.Commit]; ok {
			current = original
		}
		status := objectmodel.StatusUnresolved
		if current[request.Object] != "" {
			status = objectmodel.StatusResolved
		}
		_ = json.NewEncoder(w).Encode(objectmodel.Resolution{Status: status, Digest: current[request.Object]})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
		var request kcclient.CommitRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.commits = append(f.commits, request)
		if f.entry.CommandID == request.CommandID {
			if f.entry.Digest != string(kernel.CanonicalDigest(request.ChangeSet)) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(kernel.ErrIdempotencyConflict, "changed request")))
				return
			}
			receipt := f.receipt
			receipt.Disposition = objectmodel.DispositionReplayed
			_ = json.NewEncoder(w).Encode(receipt)
			return
		}
		if f.commitError != "" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(f.commitError, "target advanced")))
			return
		}
		f.receipt = objectmodel.CommitReceipt{
			CommandID: request.CommandID, ReceiptRef: "receipt:commit:published", Surface: "COMMIT",
			Disposition: objectmodel.DispositionApplied,
			Result: objectmodel.CommitResult{RepositoryID: initialUploadRepository,
				TargetRef: request.ChangeSet.TargetRef, OldCommit: request.ChangeSet.BaseCommit,
				CommitID: "published", NewCommit: "published"},
		}
		rawReceipt, _ := json.Marshal(f.receipt)
		cs := request.ChangeSet
		f.entry = snapshot.CommandEntry{CommandID: request.CommandID, Status: snapshot.StatusApplied,
			Digest: string(kernel.CanonicalDigest(cs)), Receipt: rawReceipt,
			Request: snapshot.CommandRequest{Kind: "COMMIT", RepositoryID: string(cs.TargetRepository),
				TargetRef: cs.TargetRef, BaseCommit: string(cs.BaseCommit), ExpectedTargetCommit: string(cs.ExpectedTargetCommit), OperationCount: len(cs.Operations)}}
		if f.advance {
			prior := map[string]kernel.Digest{}
			for id, digest := range f.current {
				prior[id] = digest
			}
			f.history[string(cs.BaseCommit)] = prior
			f.head = `{"commit":"published","empty":false}`
			for _, op := range request.ChangeSet.Operations {
				f.current[string(op.Address.ObjectID)] = kernel.CanonicalDigest(op.Value)
			}
		}
		_ = json.NewEncoder(w).Encode(f.receipt)
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/receipts/"):
		f.receipts++
		if f.receiptError != "" {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(f.receiptError, "receipt unavailable")))
			return
		}
		_ = json.NewEncoder(w).Encode(f.entry)
	default:
		http.NotFound(w, r)
	}
}

func initialUploadDirectory(t *testing.T, count int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < count; i++ {
		data := fmt.Sprintf("---\nobject_id: note/item-%d\n---\n{\"body\":\"item-%d\"}\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("item-%d.json", i)), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runInitialUpload(server *httptest.Server, dir, command string, extra ...string) RunResult {
	args := []string{"--server", server.URL, "--as", "agent:supplier", "writer", "commit",
		"--repo", initialUploadRepository, "--command-id", command, "--dir", dir}
	return Run(append(args, extra...))
}

func TestRemoteInitialUploadSkipsAddressReadsAtProvenEmptyHead(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(fmt.Sprint(bulk), func(t *testing.T) {
			f, server := newInitialUploadRemote(t, `{"commit":"empty-root","empty":true}`)
			extra := []string{"--ref", "refs/heads/import"}
			if bulk {
				extra = append(extra, "--bulk")
			}
			result := runInitialUpload(server, initialUploadDirectory(t, 8), "first", extra...)
			if result.Status != 0 {
				t.Fatal(result.Stdout)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.heads) != 0 || len(f.resolves) != 0 || len(f.commits) != 1 {
				t.Fatalf("first upload requests: heads=%v resolves=%d commits=%d", f.heads, len(f.resolves), len(f.commits))
			}
			cs := f.commits[0].ChangeSet
			if cs.BaseCommit != "" || cs.ExpectedTargetCommit != "" || cs.TargetRef != "refs/heads/import" || len(cs.Operations) != 8 || cs.BulkIngest != bulk {
				t.Fatalf("first upload lost fixed basis, operations or bulk: %#v", cs)
			}
		})
	}
}

func TestRemoteInitialUploadSendsFullIntentWithoutValueFiltering(t *testing.T) {
	for _, head := range []string{`{"commit":"existing","empty":false}`, `{"commit":"existing"}`} {
		t.Run(head, func(t *testing.T) {
			f, server := newInitialUploadRemote(t, head)
			f.current["note/item-0"] = kernel.CanonicalDigest(map[string]any{"body": "item-0"})
			result := runInitialUpload(server, initialUploadDirectory(t, 3), "incremental", "--bulk")
			if result.Status != 0 {
				t.Fatal(result.Stdout)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.heads) != 0 || len(f.resolves) != 0 || len(f.commits) != 1 {
				t.Fatalf("publication performs client comparison: heads=%d resolves=%d commits=%d", len(f.heads), len(f.resolves), len(f.commits))
			}
			cs := f.commits[0].ChangeSet
			if len(cs.Operations) != 3 || !cs.BulkIngest || cs.BaseCommit != "" || cs.ExpectedTargetCommit != "" {
				t.Fatalf("lost full intent or invented basis: %#v", cs)
			}
		})
	}
}

func TestRemoteInitialUploadExplicitBaseKeepsHistoricalDiff(t *testing.T) {
	f, server := newInitialUploadRemote(t, `{"commit":"empty-head","empty":true}`)
	result := runInitialUpload(server, initialUploadDirectory(t, 2), "historical", "--base", "historical")
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.heads) != 0 || len(f.resolves) != 0 || len(f.commits) != 1 {
		t.Fatalf("explicit base used unrelated HEAD: heads=%v resolves=%d commits=%d", f.heads, len(f.resolves), len(f.commits))
	}
	for _, request := range f.resolves {
		if request.Commit != "historical" {
			t.Fatalf("historical diff changed basis: %#v", request)
		}
	}
	cs := f.commits[0].ChangeSet
	if cs.BaseCommit != "historical" || cs.ExpectedTargetCommit != "historical" {
		t.Fatalf("historical CAS changed basis: %#v", cs)
	}
}

func TestRemoteInitialUploadHeadFailureNeverAssumesEmpty(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"forbidden", `{"error":{"code":"FORBIDDEN","message":"denied"}}`, http.StatusForbidden, "FORBIDDEN"},
		{"backend failure", `{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"offline"}}`, http.StatusInternalServerError, "TEMPORARY_UNAVAILABLE"},
		{"invalid JSON", `{`, 0, ""},
		{"missing commit", `{"empty":true}`, 0, ""},
		{"invalid commit type", `{"commit":5,"empty":true}`, 0, ""},
		{"invalid empty type", `{"commit":"root","empty":"true"}`, 0, ""},
		{"null empty", `{"commit":"root","empty":null}`, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, server := newInitialUploadRemote(t, tc.body)
			f.headStatus = tc.status
			result := Run([]string{"--server", server.URL, "--as", "agent:supplier", "diff", "--repo", initialUploadRepository, "--dir", initialUploadDirectory(t, 1)})
			if result.Status == 0 || (tc.code != "" && !strings.Contains(result.Stdout, tc.code)) {
				t.Fatalf("head failure changed meaning: %#v", result)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.resolves) != 0 || len(f.commits) != 0 {
				t.Fatalf("failed head continued upload: resolves=%d commits=%d", len(f.resolves), len(f.commits))
			}
		})
	}
}

func TestRemoteInitialUploadRejectsConcurrentAdvanceWithoutRebase(t *testing.T) {
	f, server := newInitialUploadRemote(t, `{"commit":"empty-root","empty":true}`)
	f.commitError = kernel.ErrNonFastForward
	result := runInitialUpload(server, initialUploadDirectory(t, 2), "concurrent", "--bulk")
	if result.Status == 0 || !strings.Contains(result.Stdout, "NON_FAST_FORWARD") {
		t.Fatalf("concurrent advance was not rejected: %#v", result)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.heads) != 0 || len(f.resolves) != 0 || len(f.commits) != 1 || f.receipts != 0 {
		t.Fatalf("conflict retried or rebased: heads=%d resolves=%d commits=%d receipts=%d", len(f.heads), len(f.resolves), len(f.commits), f.receipts)
	}
	cs := f.commits[0].ChangeSet
	if cs.BaseCommit != "" || cs.ExpectedTargetCommit != "" {
		t.Fatalf("concurrent upload did not pin the empty root: %#v", cs)
	}
}

func TestRemoteInitialUploadSameDirectoryReplaysOriginalReceipt(t *testing.T) {
	f, server := newInitialUploadRemote(t, `{"commit":"empty-root","empty":true}`)
	f.advance = true
	dir := initialUploadDirectory(t, 2)
	first := runInitialUpload(server, dir, "retry-first", "--bulk")
	second := runInitialUpload(server, dir, "retry-first", "--bulk")
	if first.Status != 0 || second.Status != 0 {
		t.Fatalf("directory retry failed: first=%s second=%s", first.Stdout, second.Stdout)
	}
	var receipt objectmodel.CommitReceipt
	if err := json.Unmarshal([]byte(second.Stdout), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.CommandID != "retry-first" || receipt.Disposition != objectmodel.DispositionReplayed || receipt.Result.NewCommit != "published" {
		t.Fatalf("retry returned a different outcome: %#v", receipt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commits) != 2 || f.receipts != 0 || f.commits[1].ChangeSet.BaseCommit != "" || f.commits[0].ChangeSet.TargetRef != snapshot.DefaultRef {
		t.Fatalf("directory retry did not verify the original request: commits=%d receipts=%d", len(f.commits), f.receipts)
	}
}

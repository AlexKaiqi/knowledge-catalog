package cli

import (
	"encoding/json"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRemoteDirectoryCommitTransmitsBulkAfterFilteringUnchanged(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "bulk"}[bulk], func(t *testing.T) {
			dir := remoteBulkDirectory(t)
			var commits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/object/v1/objects:resolve":
					var request struct {
						Object string `json:"object"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					digest := "old-value"
					if request.Object == "notes/same" {
						digest = string(kernel.CanonicalDigest(map[string]any{"v": 1}))
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "RESOLVED", "digest": digest})
				case strings.HasSuffix(r.URL.Path, "/commits"):
					commits.Add(1)
					var request struct {
						CommandID string                      `json:"commandId"`
						ChangeSet objectmodel.CommitChangeSet `json:"changeSet"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.ChangeSet.BulkIngest != bulk {
						t.Errorf("HTTP directory commit bulkIngest=%v, want %v", request.ChangeSet.BulkIngest, bulk)
					}
					if len(request.ChangeSet.Operations) != 1 || request.ChangeSet.Operations[0].Address.ObjectID != "notes/changed" {
						t.Errorf("HTTP commit did not retain only changed directory content: %#v", request.ChangeSet.Operations)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"commandId": request.CommandID, "disposition": "APPLIED"})
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			args := []string{"writer", "commit", "--repo", "kr://test/bulk-directory", "--command-id", "directory-import", "--dir", dir, "--base", "fixed"}
			if bulk {
				args = append(args, "--bulk")
			}
			result := runBulkDirectoryCLI(t, server.URL, args...)
			if result.Status != 0 || commits.Load() != 1 {
				t.Fatalf("directory commit: status=%d calls=%d %s", result.Status, commits.Load(), result.Stdout)
			}
		})
	}
}

type directoryStagingOnlyStore struct {
	snapshot.Store
	snapshot.TreeStore
}

func TestRemoteDirectoryBulkPreservesUnsupportedMediumError(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/no-bulk")
	registry := snapshot.NewRegistry()
	if err := registry.Add(&directoryStagingOnlyStore{Store: repo, TreeStore: repo}); err != nil {
		t.Fatal(err)
	}
	commands, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, commands)
	if err != nil {
		t.Fatal(err)
	}
	basis := testkit.MustHead(t, repo, snapshot.DefaultRef)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, r *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/object/v1/objects:resolve" {
			_ = json.NewEncoder(response).Encode(map[string]any{"status": "UNRESOLVED"})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/commits") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(response, r)
			return
		}
		var request struct {
			CommandID string                      `json:"commandId"`
			ChangeSet objectmodel.CommitChangeSet `json:"changeSet"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		receipt, err := w.Commit(request.CommandID, request.ChangeSet)
		if err != nil {
			response.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(response).Encode(kernel.FaultJSON(err))
			return
		}
		_ = json.NewEncoder(response).Encode(receipt)
	}))
	t.Cleanup(server.Close)
	result := runBulkDirectoryCLI(t, server.URL, "writer", "commit", "--repo", string(repo.ID()), "--command-id", "no-bulk", "--dir", remoteBulkDirectory(t), "--base", string(basis), "--bulk")
	if result.Status == 0 || !strings.Contains(result.Stdout, string(kernel.ErrCapabilityUnsatisfied)) {
		t.Fatalf("unsupported bulk medium must be reported without ordinary fallback: %#v", result)
	}
	if got := testkit.MustHead(t, repo, snapshot.DefaultRef); got != basis {
		t.Fatalf("unsupported bulk changed target: %s -> %s", basis, got)
	}
}

func remoteBulkDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"same.json":    "---\nobject_id: notes/same\n---\n{\"v\":1}\n",
		"changed.json": "---\nobject_id: notes/changed\n---\n{\"v\":2}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runBulkDirectoryCLI(t *testing.T, server string, args ...string) RunResult {
	t.Helper()
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_DATASET", "")
	return Run(append([]string{"--server", server, "--as", "agent:directory-import"}, args...))
}

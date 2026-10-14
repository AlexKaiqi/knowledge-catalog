package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	snapshotclient "kc/snapshot/client"
)

func TestSnapshotArgvPublishesMetadataAndKeepsHostActionPrivate(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Kc-As") != "alice" || r.Header.Get("X-Kc-Request-Id") != "publish-A" {
			t.Errorf("module lost the host identity or request context: %#v", r.Header)
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /snapshot/v1/datasets":
			var request snapshotclient.DatasetRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Dataset != "notes" || request.Revision != 1 || request.Metadata["description"] != "tea" || len(request.Sources) != 1 {
				t.Errorf("argv definition lost metadata or mappings: %#v", request)
			}
		case "GET /snapshot/v1/datasets/notes":
			if r.URL.Query().Get("revision") != "1" {
				t.Errorf("argv selection lost its fixed revision: %s", r.URL)
			}
		default:
			t.Errorf("unexpected Snapshot operation: %s %s", r.Method, r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "notes", "revision": 1})
	}))
	defer server.Close()
	for _, args := range [][]string{
		{"dataset", "define", "notes", "--revision", "1", "--source", "repo-A@reference", "--metadata", `{"description":"tea"}`},
		{"dataset", "show", "notes", "--revision", "1"},
	} {
		result := Run(append(args, "--server", server.URL, "--as", "alice", "--request-id", "publish-A"))
		if result.Status != 0 {
			t.Fatalf("public argv did not reach the composed module: %#v", result)
		}
	}
	if calls != 2 {
		t.Fatalf("argv did not perform both typed operations: %d", calls)
	}
}

func TestDatasetMetadataFlagCannotExpandOtherCommandInputs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	for _, command := range [][]string{{"writer", "remove"}, {"search"}, {"dataset", "retire", "notes"}, {"grant", "add"}} {
		argv := append(command, "--server", server.URL, "--as", "alice", "--metadata", `{}`)
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "unknown flag --metadata") {
			t.Fatalf("metadata broadened another command's existing vocabulary: %v %#v", command, result)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid metadata scope issued %d requests", calls)
	}
}

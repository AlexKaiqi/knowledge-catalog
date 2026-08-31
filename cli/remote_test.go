package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteWriterIngestGetsBaseFromServerWithoutOpeningHome(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runbook.json"), []byte(`{"body":"recover"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/head") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"commit": "base-1"})
	}))
	t.Cleanup(server.Close)
	out := filepath.Join(t.TempDir(), "preview.json")
	result := Run([]string{"--server", server.URL, "--as", "agent:test", "writer", "ingest",
		"--repo", "kr://acme/core", "--dir", dir, "--out", out})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if raw, err := os.ReadFile(out); err != nil || !strings.Contains(string(raw), `"baseCommit": "base-1"`) {
		t.Fatalf("remote preview did not persist the server-derived base: %v %s", err, raw)
	}
}

func TestRemoteGroupedCLIUsesTypedKnowledgeClient(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/knowledge/v1/objects:read" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"objectId": request["object"]}})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	t.Setenv("KC_HOME", t.TempDir())
	result := Run([]string{"knowledge", "read", "--workspace", "agent", "--object", "policy/A"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["workspace"] != "agent" || request["object"] != "policy/A" {
		t.Fatalf("typed request %#v", request)
	}
}

func TestRemoteCLIUsesBoundCatalogAndWorkspaceEnvironment(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/knowledge/v1/objects:read" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode(map[string]any{"objectId": request["object"]})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	t.Setenv("KC_CATALOG", "kr://acme/catalog")
	t.Setenv("KC_WORKSPACE", "agent")
	result := Run([]string{"knowledge", "read", "--object", "policy/A"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != "kr://acme/catalog" || request["workspace"] != "agent" {
		t.Fatalf("typed request did not inherit bound context: %#v", request)
	}
}

func TestRemoteSearchPreservesEveryPublicOperator(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/knowledge/v1/search" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode(map[string]any{"searchView": map[string]any{"snapshots": map[string]any{}}, "completeness": "complete", "hits": []any{}})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	result := Run([]string{
		"knowledge", "search", "--workspace", "agent", "--query", "runbook",
		"--in", "owner=a,b", "--exists", "active", "--missing", "deleted",
		"--prefix", "name=customer.", "--gt", "score=1", "--gte", "score=2",
		"--lt", "score=9", "--lte", "score=8",
	})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	for field, want := range map[string]string{
		"in": "owner=a,b", "exists": "active", "missing": "deleted", "prefix": "name=customer.",
		"greaterThan": "score=1", "greaterEqual": "score=2", "lessThan": "score=9", "lessEqual": "score=8",
	} {
		values, ok := request[field].([]any)
		if !ok || len(values) != 1 || values[0] != want {
			t.Fatalf("%s = %#v, want [%q]", field, request[field], want)
		}
	}
}

func TestRemoteCLIRejectsHomeAndMissingPrincipal(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	t.Setenv("KC_AS", "")
	result := Run([]string{"--server", server.URL, "--home", t.TempDir(), "identity", "whoami"})
	if result.Status == 0 {
		t.Fatal("remote mode accepted --home")
	}
	result = Run([]string{"--server", server.URL, "identity", "whoami"})
	if result.Status == 0 {
		t.Fatal("remote mode accepted an implicit owner")
	}
}

func TestLocalGroupNeverRoutesThroughServerDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("kc local command reached HTTP")
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	home := t.TempDir()
	result := Run([]string{"--home", home, "local", "init", "--catalog", "kr://local/catalog"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	result = Run([]string{"--server", server.URL, "local", "status"})
	if result.Status == 0 {
		t.Fatal("kc local accepted --server")
	}
}

func TestProductCommandsRequireServer(t *testing.T) {
	t.Setenv("KC_SERVER_URL", "")
	for _, argv := range [][]string{
		{"knowledge", "search", "--workspace", "agent", "--query", "runbook"},
		{"catalog", "show", "--catalog", "kr://acme/catalog"},
		{"writer", "put", "--repo", "kr://acme/core", "--command-id", "c1", "--object", "Policy:x", "--value", `{}`},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "requires KC Server") {
			t.Fatalf("%v bypassed KC Server: %s", argv, result.Stdout)
		}
	}
}

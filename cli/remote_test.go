package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/snapshot"
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
	if _, ok := request["repository"]; ok {
		t.Fatalf("workspace read must not send a repository pin: %#v", request)
	}
}

func TestRemoteKnowledgeReadUsesRepositoryBasisWithoutWorkspace(t *testing.T) {
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
	t.Setenv("KC_AS", "agent:provider")
	t.Setenv("KC_WORKSPACE", "agent")
	result := Run([]string{"knowledge", "read", "--repo", "kr://acme/core", "--object", "runbook/payment-oncall"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["repository"] != "kr://acme/core" || request["object"] != "runbook/payment-oncall" {
		t.Fatalf("typed repository read %#v", request)
	}
	if _, ok := request["workspace"]; ok {
		t.Fatalf("repository read must not send a Workspace: %#v", request)
	}
}

func TestRemoteCatalogListDoesNotRequireCatalogID(t *testing.T) {
	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []any{map[string]any{"id": "kr://acme/catalog"}}})
	}))
	t.Cleanup(server.Close)
	result := Run([]string{"--server", server.URL, "--as", "agent:consumer", "catalog", "list"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if path := <-seen; path != "/catalog/v1/catalogs" {
		t.Fatalf("catalog list path = %s", path)
	}
}

func TestRemoteCatalogShowInfersSingleCatalog(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/catalog/v1/catalogs" {
			_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []any{map[string]any{"id": "kr://acme/catalog"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"catalogId": "kr://acme/catalog"})
	}))
	t.Cleanup(server.Close)
	result := Run([]string{"--server", server.URL, "--as", "agent:consumer", "catalog", "show"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if len(paths) != 2 || paths[0] != "/catalog/v1/catalogs" || paths[1] == "/catalog/v1/catalogs" || !strings.Contains(paths[1], "acme") {
		t.Fatalf("catalog show inference paths = %v", paths)
	}
}

func TestRemoteWorkspaceResolveUsesTemporaryDefinition(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/workspaces/resolve") || strings.Contains(r.URL.Path, "/workspaces/agent/") {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode(map[string]any{"pinId": "pin-1", "repositories": map[string]any{"kr://acme/core": "c1"}})
	}))
	t.Cleanup(server.Close)
	result := Run([]string{"--server", server.URL, "--as", "agent:consumer",
		"catalog", "workspace", "resolve", "--catalog", "kr://acme/catalog",
		"--source", "kr://acme/core"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	sources, _ := request["sources"].([]any)
	if len(sources) != 1 || asMapValue(sources[0])["repository"] != "kr://acme/core" {
		t.Fatalf("temporary resolve body %#v", request)
	}
	if asMapValue(sources[0])["selector"] != snapshot.DefaultRef {
		t.Fatalf("id-only --source must fill the published selector: %#v", request)
	}
	if request["workspace"] != nil && request["workspace"] != "" {
		t.Fatalf("temporary resolve must not invent a named knowledge set: %#v", request)
	}
}

func asMapValue(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
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

func TestRemoteGrantAddDoesNotInheritBoundWorkspace(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/admin/v1/grants" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "alw_1"})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "service:bootstrap")
	t.Setenv("KC_CATALOG", "kr://acme/catalog")
	t.Setenv("KC_WORKSPACE", "warehouse-agent")
	result := Run([]string{"admin", "grant", "add", "--principal", "agent:dsh",
		"--action", "catalog.read", "--catalog", "kr://acme/catalog"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != "kr://acme/catalog" || request["principal"] != "agent:dsh" {
		t.Fatalf("grant request %#v", request)
	}
	if workspace, ok := request["workspace"]; ok && workspace != nil && workspace != "" {
		t.Fatalf("grant add must not inherit KC_WORKSPACE: %#v", request)
	}
}

func TestRemoteAccessDescribeSendsWorkspaceNotRepository(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/operations/v1/access-specs:describe" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen <- request
		_ = json.NewEncoder(w).Encode(map[string]any{"workspaceId": "agent", "specs": []any{}})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "service:bootstrap")
	t.Setenv("KC_CATALOG", "kr://acme/catalog")
	t.Setenv("KC_WORKSPACE", "agent")
	result := Run([]string{"operations", "access", "describe"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != "kr://acme/catalog" || request["workspace"] != "agent" {
		t.Fatalf("access describe must send the Workspace pin: %#v", request)
	}
	if _, ok := request["repository"]; ok {
		t.Fatalf("access describe must not send a Repository projection coordinate: %#v", request)
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

func TestRemoteCLILocalLoginPersistsPrincipal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KC_AS", "")
	var gotAs string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/v1/auth":
			_ = json.NewEncoder(w).Encode(map[string]any{"mode": "local", "localAssertion": true, "accepts": []string{"X-Kc-As"}})
		case "/identity/v1/whoami":
			gotAs = r.Header.Get("X-Kc-As")
			_ = json.NewEncoder(w).Encode(map[string]any{"principal": gotAs})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	login := Run([]string{"login", "--server", server.URL, "--mode", "local", "--as", "agent:dsh"})
	if login.Status != 0 {
		t.Fatal(login.Stdout)
	}
	who := Run([]string{"--server", server.URL, "identity", "whoami"})
	if who.Status != 0 {
		t.Fatal(who.Stdout)
	}
	if gotAs != "agent:dsh" {
		t.Fatalf("local login must send X-Kc-As from the persisted session, got %q", gotAs)
	}
	logout := Run([]string{"logout", "--server", server.URL})
	if logout.Status != 0 {
		t.Fatal(logout.Stdout)
	}
	missing := Run([]string{"--server", server.URL, "identity", "whoami"})
	if missing.Status == 0 {
		t.Fatal("logout must clear the persisted local principal")
	}
}

func TestRemoteCLITokenLoginSendsAuthorizationOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KC_AS", "")
	t.Setenv("KC_AUTH_TOKEN", "")
	var whoami http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/v1/auth":
			_ = json.NewEncoder(w).Encode(map[string]any{"mode": "taihu", "localAssertion": false, "accepts": []string{"Authorization"}})
		case "/identity/v1/whoami":
			whoami = r.Header.Clone()
			_ = json.NewEncoder(w).Encode(map[string]any{"principal": "taihu:stub"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	login := Run([]string{"login", "--server", server.URL, "--mode", "token", "--token", "test-token"})
	if login.Status != 0 {
		t.Fatal(login.Stdout)
	}
	who := Run([]string{"--server", server.URL, "identity", "whoami"})
	if who.Status != 0 {
		t.Fatal(who.Stdout)
	}
	if whoami.Get("Authorization") != "Bearer test-token" || whoami.Get("X-Kc-As") != "" {
		t.Fatalf("token pairing must send Authorization only: %v", whoami)
	}
	mixed := Run([]string{"--server", server.URL, "--as", "agent:forged", "identity", "whoami"})
	if mixed.Status == 0 || !strings.Contains(mixed.Stdout, "USAGE_INVALID") {
		t.Fatalf("token + --as must fail closed: %#v", mixed)
	}
}

func TestRemoteCLIRejectsHomeAndMissingPrincipal(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	t.Setenv("HOME", t.TempDir())
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

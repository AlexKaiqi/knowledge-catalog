package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

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
	result := Run([]string{"read", "--dataset", "agent", "--object", "policy/A"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["dataset"] != "agent" || request["object"] != "policy/A" {
		t.Fatalf("typed request %#v", request)
	}
	if _, ok := request["repository"]; ok {
		t.Fatalf("dataset read must not send a repository coordinate: %#v", request)
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
	t.Setenv("KC_WORKSPACE", "")
	result := Run([]string{"read", "--repo", "kr://acme/core", "--object", "runbook/payment-oncall"})
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
	isolateLoginConfig(t)
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
	isolateLoginConfig(t)
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
	result := Run([]string{"--server", server.URL, "--as", "agent:consumer", "show"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if len(paths) != 2 || paths[0] != "/catalog/v1/catalogs" || paths[1] == "/catalog/v1/catalogs" || !strings.Contains(paths[1], "acme") {
		t.Fatalf("catalog show inference paths = %v", paths)
	}
}

func TestRemoteWorkspacePinStaysRemoved(t *testing.T) {
	isolateLoginConfig(t)
	result := Run([]string{"workspace", "pin", "--source", "kr://acme/core"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
		t.Fatalf("workspace pin must stay removed from the product CLI: %s", result.Stdout)
	}
}

func asMapValue(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func TestRemoteCLIUsesBoundCatalogAndWorkspaceEnvironment(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/catalog/v1/catalogs" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []map[string]string{{"id": "kr://acme/catalog"}}})
			return
		}
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
	isolateLoginConfig(t)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	if use := Run([]string{"catalog", "use", "kr://acme/catalog"}); use.Status != 0 {
		t.Fatal(use.Stdout)
	}
	result := Run([]string{"read", "--repo", "kr://acme/core", "--object", "policy/A"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != "kr://acme/catalog" || request["repository"] != "kr://acme/core" {
		t.Fatalf("typed request did not inherit bound catalog and repository: %#v", request)
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
	result := Run([]string{"grant", "add", "--principal", "agent:dsh",
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

func TestRemoteAccessDescribeSendsThePinnedWorkspace(t *testing.T) {
	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/catalog/v1/catalogs" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []map[string]string{{"id": "kr://acme/catalog"}}})
			return
		}
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
	isolateLoginConfig(t)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "service:bootstrap")
	if use := Run([]string{"catalog", "use", "kr://acme/catalog"}); use.Status != 0 {
		t.Fatal(use.Stdout)
	}
	result := Run([]string{"operations", "access-spec", "describe", "--dataset", "agent"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != "kr://acme/catalog" || request["dataset"] != "agent" {
		t.Fatalf("access describe must send the named Dataset: %#v", request)
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
		"search", "--repo", "kr://acme/core", "--query", "runbook",
		"--in", "owner=a,b", "--exists", "active", "--missing", "deleted",
		"--prefix", "name=customer.", "--contains", "name=tomer", "--gt", "score=1", "--gte", "score=2",
		"--lt", "score=9", "--lte", "score=8",
	})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	for field, want := range map[string]string{
		"in": "owner=a,b", "exists": "active", "missing": "deleted", "prefix": "name=customer.",
		"contains":    "name=tomer",
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
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
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
	who := Run([]string{"--server", server.URL, "whoami"})
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
	missing := Run([]string{"--server", server.URL, "whoami"})
	if missing.Status == 0 {
		t.Fatal("logout must clear the persisted local principal")
	}
}

func TestRemoteCLITokenLoginSendsAuthorizationOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KC_AS", "")
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
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
	who := Run([]string{"--server", server.URL, "whoami"})
	if who.Status != 0 {
		t.Fatal(who.Stdout)
	}
	if whoami.Get("Authorization") != "Bearer test-token" || whoami.Get("X-Kc-As") != "" {
		t.Fatalf("token pairing must send Authorization only: %v", whoami)
	}
	mixed := Run([]string{"--server", server.URL, "--as", "agent:forged", "whoami"})
	if mixed.Status == 0 || !strings.Contains(mixed.Stdout, "USAGE_INVALID") {
		t.Fatalf("token + --as must fail closed: %#v", mixed)
	}
	logout := Run([]string{"logout", "--server", server.URL})
	if logout.Status != 0 {
		t.Fatal(logout.Stdout)
	}
	missing := Run([]string{"--server", server.URL, "whoami"})
	if missing.Status == 0 {
		t.Fatal("logout must clear the persisted token session")
	}
}

func TestRemoteCLIRejectsHomeAndMissingPrincipal(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KC_AS", "")
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	result := Run([]string{"--server", server.URL, "--home", t.TempDir(), "whoami"})
	if result.Status == 0 {
		t.Fatal("remote mode accepted --home")
	}
	result = Run([]string{"--server", server.URL, "whoami"})
	if result.Status == 0 {
		t.Fatal("remote mode accepted an implicit owner")
	}
}

func TestRetiredLocalGroupNeverRoutesThroughServerDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("kc local command reached HTTP")
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	home := t.TempDir()
	result := Run([]string{"--home", home, "local", "init", "--catalog", "kr://local/catalog"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
		t.Fatal(result.Stdout)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("retired local command wrote state: %v %v", entries, err)
	}
	result = Run([]string{"--server", server.URL, "local", "status"})
	if result.Status == 0 {
		t.Fatal("kc local accepted --server")
	}
}

func TestProductCommandsRequireServer(t *testing.T) {
	isolateLoginConfig(t)
	t.Setenv("KC_SERVER_URL", "")
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"search", "--workspace", "agent", "--query", "runbook"}, "USAGE_INVALID"},
		{[]string{"show"}, "requires KC Server"},
		{[]string{"writer", "put", "--repo", "kr://acme/core", "--command-id", "c1", "--object", "Policy:x", "--value", `{}`}, "requires KC Server"},
	} {
		result := Run(tc.argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, tc.want) {
			t.Fatalf("%v: want %q got %s", tc.argv, tc.want, result.Stdout)
		}
	}
}

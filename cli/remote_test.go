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
		_ = json.NewEncoder(w).Encode(map[string]any{"reference": request["reference"], "repository": "kr://acme/core", "address": map[string]any{"objectId": "policy/A"}})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	t.Setenv("KC_HOME", t.TempDir())
	result := Run([]string{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fcore:policy%2FA", "--dataset", "agent"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["dataset"] != "agent" || request["reference"] != "urn:kc:object:kr%3A%2F%2Facme%2Fcore:policy%2FA" {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"reference": request["reference"]})
	}))
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:provider")
	t.Setenv("KC_WORKSPACE", "")
	result := Run([]string{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fcore:runbook%2Fpayment-oncall"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["reference"] != "urn:kc:object:kr%3A%2F%2Facme%2Fcore:runbook%2Fpayment-oncall" {
		t.Fatalf("typed repository read %#v", request)
	}
	if _, ok := request["workspace"]; ok {
		t.Fatalf("repository read must not send a Workspace: %#v", request)
	}
}

func TestRemoteInventoryDoesNotDiscoverOrSelectCatalog(t *testing.T) {
	isolateLoginConfig(t)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []any{}, "datasets": []any{}})
	}))
	t.Cleanup(server.Close)
	result := Run([]string{"--server", server.URL, "--as", "agent:consumer", "show"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if len(paths) != 1 || paths[0] != "/snapshot/v1/inventory" {
		t.Fatalf("inventory performed retired discovery: %v", paths)
	}
	rejected := Run([]string{"--server", server.URL, "--as", "agent:consumer", "catalog", "list"})
	if rejected.Status == 0 || !strings.Contains(rejected.Stdout, "USAGE_INVALID") || len(paths) != 1 {
		t.Fatalf("retired listing reached HTTP: %#v %v", rejected, paths)
	}
}

func TestRemoteWorkspacePinStaysRemoved(t *testing.T) {
	isolateLoginConfig(t)
	result := Run([]string{"workspace", "pin", "--source", "kr://acme/core"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
		t.Fatalf("workspace pin must stay removed from the product CLI: %s", result.Stdout)
	}
}

func TestRemoteCLIUsesRepositoryWithoutCatalogContext(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"reference": request["reference"]})
	}))
	t.Cleanup(server.Close)
	isolateLoginConfig(t)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AS", "agent:test")
	result := Run([]string{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fcore:policy%2FA"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != nil || request["reference"] != "urn:kc:object:kr%3A%2F%2Facme%2Fcore:policy%2FA" {
		t.Fatalf("typed request did not send only the explicit repository: %#v", request)
	}
}

func TestRemoteGrantAddDoesNotInheritRetiredCatalogOrBoundWorkspace(t *testing.T) {
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
		"--action", "inventory.read"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != nil || request["principal"] != "agent:dsh" {
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
	result := Run([]string{"operations", "access-spec", "describe", "--dataset", "agent"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	request := <-seen
	if request["catalog"] != nil || request["dataset"] != "agent" {
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
	result := Run([]string{"--home", home, "local", "init"})
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
		{[]string{"writer", "put", "urn:kc:object:kr%3A%2F%2Facme%2Fcore:Policy%3Ax", "--command-id", "c1", "--value", `{}`}, "requires KC Server"},
	} {
		result := Run(tc.argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, tc.want) {
			t.Fatalf("%v: want %q got %s", tc.argv, tc.want, result.Stdout)
		}
	}
}

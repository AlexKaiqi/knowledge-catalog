package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

type publicHTTPRoute struct {
	Method  string
	Pattern string
}

var routeRegistration = regexp.MustCompile(`mux\.HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"`)

// TestEveryPublicHTTPRouteIsRegisteredWithOnlyItsDeclaredMethod derives the
// denominator from the production registration sites. It deliberately does
// not share the CLI command table: HTTP is an independent typed protocol.
func TestEveryPublicHTTPRouteIsRegisteredWithOnlyItsDeclaredMethod(t *testing.T) {
	routes := registeredHTTPRoutes(t)
	if len(routes) != 65 {
		t.Fatalf("public HTTP route count changed from the reviewed 65 to %d; review the new protocol surface", len(routes))
	}

	handler := cli.HTTPHandlerWithOptions(testkit.TempDir(t), cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	seen := map[string]struct{}{}
	for _, route := range routes {
		key := route.Method + " " + route.Pattern
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate HTTP route registration %s", key)
		}
		seen[key] = struct{}{}
		if !allowedHTTPPath(route.Pattern) {
			t.Errorf("route escaped the formal namespaces: %s", key)
			continue
		}

		t.Run(strings.NewReplacer("/", "_", "{", "", "}", "", ":", "_").Replace(key), func(t *testing.T) {
			path := concreteHTTPPath(route.Pattern)
			var payload *bytes.Reader
			if route.Method == http.MethodGet {
				payload = bytes.NewReader(nil)
			} else {
				payload = bytes.NewReader([]byte("{}"))
			}
			request, err := http.NewRequest(route.Method, server.URL+path, payload)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Kc-As", "agent:http-surface")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
				t.Fatalf("declared route is unreachable: %s returned %d", key, response.StatusCode)
			}

			undeclared, err := http.NewRequest(http.MethodPatch, server.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			undeclared.Header.Set("X-Kc-As", "agent:http-surface")
			wrong, err := server.Client().Do(undeclared)
			if err != nil {
				t.Fatal(err)
			}
			_ = wrong.Body.Close()
			if wrong.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("undeclared PATCH %s returned %d, want 405", path, wrong.StatusCode)
			}
		})
	}
}

// TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses covers the
// endpoints which have no remote CLI dispatcher. The three Workspace File
// routes have their richer fixed-pin/read scenario in workspace_file_service_test.go.
func TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/http-only/catalog"
	repositoryID := "kr://acme/http-only/repository"
	principal := "agent:http-only"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))
	body(t, kc(home, "catalog", "repo", "register", "--repo", repositoryID))
	body(t, kc(home, "writer", "put", "--command-id", "http-only-seed", "--repo", repositoryID,
		"--object", "Policy:http-only", "--value", `{"body":"http-only"}`))
	body(t, kc(home, "workspace", "define", "--workspace", "agent", "--revision", "1",
		"--source", repositoryID+"=refs/heads/main@knowledge"))
	body(t, kc(home, "admin", "grant", "add", "--principal", principal,
		"--action", "catalog.read,workspace.resolve", "--catalog", catalogID))
	body(t, kc(home, "admin", "grant", "add", "--principal", principal,
		"--action", "workspace.consume", "--catalog", catalogID, "--workspace", "agent"))
	body(t, kc(home, "admin", "grant", "add", "--principal", principal,
		"--action", "knowledge.read,governance.proposal.create", "--repo", repositoryID))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	for _, endpoint := range []struct {
		path   string
		status map[int]bool
		field  string
		value  any
	}{
		{"/health", map[int]bool{http.StatusOK: true}, "ok", true},
		{"/livez", map[int]bool{http.StatusOK: true}, "status", "live"},
		{"/readyz", map[int]bool{http.StatusOK: true, http.StatusServiceUnavailable: true}, "status", nil},
		{"/readyz/consumer", map[int]bool{http.StatusOK: true}, "surface", "consumer"},
	} {
		status, payload, _ := httpSurfaceRequest(t, server, http.MethodGet, endpoint.path, nil, principal)
		if !endpoint.status[status] {
			t.Fatalf("GET %s returned %d: %#v", endpoint.path, status, payload)
		}
		object := asMap(t, payload)
		if _, exists := object[endpoint.field]; !exists {
			t.Fatalf("GET %s omitted %s: %#v", endpoint.path, endpoint.field, object)
		}
		if endpoint.value != nil && object[endpoint.field] != endpoint.value {
			t.Fatalf("GET %s %s=%#v, want %#v", endpoint.path, endpoint.field, object[endpoint.field], endpoint.value)
		}
	}

	status, _, contentType := httpSurfaceRequest(t, server, http.MethodGet, "/metrics", nil, principal)
	if status != http.StatusOK || !strings.Contains(contentType, "text/plain") {
		t.Fatalf("GET /metrics returned %d content-type %q", status, contentType)
	}
	status, identity, _ := httpSurfaceRequest(t, server, http.MethodGet, "/identity/v1/whoami", nil, principal)
	if status != http.StatusOK || asMap(t, identity)["principal"] != principal {
		t.Fatalf("GET /identity/v1/whoami returned %d %#v", status, identity)
	}
	authReq, err := http.NewRequest(http.MethodGet, server.URL+"/identity/v1/auth", nil)
	if err != nil {
		t.Fatal(err)
	}
	authResp, err := server.Client().Do(authReq)
	if err != nil {
		t.Fatal(err)
	}
	authRaw, err := io.ReadAll(authResp.Body)
	authResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var discovered map[string]any
	if authResp.StatusCode != http.StatusOK || json.Unmarshal(authRaw, &discovered) != nil || discovered["mode"] != "local" || discovered["localAssertion"] != true {
		t.Fatalf("unauthenticated GET /identity/v1/auth returned %d %s", authResp.StatusCode, authRaw)
	}
	status, discoveredAny, _ := httpSurfaceRequest(t, server, http.MethodGet, "/identity/v1/auth", nil, principal)
	if status != http.StatusOK || asMap(t, discoveredAny)["mode"] != "local" {
		t.Fatalf("GET /identity/v1/auth with X-Kc-As returned %d %#v", status, discoveredAny)
	}
	status, catalogs, _ := httpSurfaceRequest(t, server, http.MethodGet, "/catalog/v1/catalogs", nil, principal)
	listed := asMap(t, catalogs)["catalogs"].([]any)
	if status != http.StatusOK || len(listed) != 1 || asMap(t, listed[0])["id"] != catalogID {
		t.Fatalf("GET /catalog/v1/catalogs returned %d %#v", status, catalogs)
	}
	if _, ok := asMap(t, listed[0])["dir"]; ok {
		t.Fatalf("catalog inventory must not leak host paths: %#v", listed[0])
	}

	catalogPath := "/catalog/v1/catalogs/" + url.PathEscape(catalogID)
	status, resolved, _ := httpSurfaceRequest(t, server, http.MethodPost, catalogPath+"/workspaces:resolve", map[string]any{
		"workspace": "adhoc", "revision": 1,
		"sources": []map[string]any{{"repository": repositoryID, "selector": "refs/heads/main"}},
	}, principal)
	if status != http.StatusOK || asMap(t, resolved)["workspaceId"] != "adhoc" {
		t.Fatalf("resolve arbitrary Workspace definition returned %d %#v", status, resolved)
	}
}

func httpSurfaceRequest(t *testing.T, server *httptest.Server, method, path string, payload any, principal string) (int, any, string) {
	t.Helper()
	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		bodyReader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, server.URL+path, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Kc-As", principal)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s %s returned invalid JSON: %v: %s", method, path, err, raw)
		}
	}
	return response.StatusCode, decoded, response.Header.Get("Content-Type")
}

func registeredHTTPRoutes(t *testing.T) []publicHTTPRoute {
	t.Helper()
	routes := []publicHTTPRoute{}
	for _, name := range []string{"serve_facade.go", "service_routes.go", "service_management_routes.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range routeRegistration.FindAllStringSubmatch(string(raw), -1) {
			routes = append(routes, publicHTTPRoute{Method: match[1], Pattern: match[2]})
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Pattern == routes[j].Pattern {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Pattern < routes[j].Pattern
	})
	return routes
}

func concreteHTTPPath(pattern string) string {
	replacer := strings.NewReplacer(
		"{catalog}", "kr:%2F%2Facme%2Fcatalog",
		"{repository}", "kr:%2F%2Facme%2Frepository",
		"{workspace}", "agent",
		"{command}", "command-1",
		"{grant}", "grant-1",
		"{hook}", "hook-1",
		"{gate}", "gate-1",
		"{surface}", "consumer",
	)
	return replacer.Replace(pattern)
}

func allowedHTTPPath(path string) bool {
	for _, exact := range []string{"/health", "/livez", "/readyz", "/readyz/{surface}", "/metrics"} {
		if path == exact {
			return true
		}
	}
	for _, namespace := range []string{
		"/identity/v1/", "/catalog/v1/", "/knowledge/v1/", "/workspace-files/v1/",
		"/writer/v1/", "/governance/v1/", "/admin/v1/", "/operations/v1/",
	} {
		if strings.HasPrefix(path, namespace) {
			return true
		}
	}
	return false
}

package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"kc/internal/testkit"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type publicHTTPRoute struct {
	Method  string
	Pattern string
}

var routeRegistration = regexp.MustCompile(`mux\.Handle(?:Func)?\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"`)

// TestEveryPublicHTTPRouteIsRegisteredWithOnlyItsDeclaredMethod derives the
// denominator from the production registration sites. It deliberately does
// not share the CLI command table: HTTP is an independent typed protocol.
func TestEveryPublicHTTPRouteIsRegisteredWithOnlyItsDeclaredMethod(t *testing.T) {
	routes := registeredHTTPRoutes(t)
	if len(routes) != 90 {
		t.Fatalf("public HTTP route count changed from the reviewed 90 to %d; review the new protocol surface", len(routes))
	}

	handler := serv.HTTPHandlerWithOptions(testkit.TempDir(t), serv.HTTPServerOptions{})
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

			wrongMethod := http.MethodPatch
			if route.Method == http.MethodPatch {
				wrongMethod = http.MethodOptions
			}
			undeclared, err := http.NewRequest(wrongMethod, server.URL+path, nil)
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
				t.Fatalf("undeclared %s %s returned %d, want 405", wrongMethod, path, wrong.StatusCode)
			}
		})
	}
}

// TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses covers the
// endpoints which have no remote CLI dispatcher. The three Workspace File
// routes have their richer fixed-pin/read scenario in workspace_file_service_test.go.
func TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses(t *testing.T) {
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/http-only/repository"
	principal := "agent:http-only"
	body(t, kc(home, "local", "init"))
	repositoryDSN := lakeFSRepoDSN(t)
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID, "--dsn", repositoryDSN))
	body(t, kc(home, "attach", "--repo", repositoryID, "--dsn", repositoryDSN))
	body(t, kc(home, "writer", "put", testObjectReference(t, repositoryID, "policy/http-only", ""), "--repo", repositoryID, "--command-id", "http-only-seed", "--value", `{"body":"http-only"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", repositoryID+"=refs/heads/main@object"))
	body(t, kc(home, "grant", "add", "--principal", principal,
		"--action", "inventory.read,dataset.resolve"))
	body(t, kc(home, "grant", "add", "--principal", principal,
		"--action", "file.read", "--dataset", "agent"))
	body(t, kc(home, "grant", "add", "--principal", principal,
		"--action", "object.read,writer.branch", "--repo", repositoryID))

	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
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
	status, inventory, _ := httpSurfaceRequest(t, server, http.MethodGet, "/snapshot/v1/inventory", nil, principal)
	view := asMap(t, inventory)
	if status != http.StatusOK || !hasRepository(view, repositoryID) {
		t.Fatalf("GET inventory: %d %#v", status, inventory)
	}
	if _, present := view["catalogId"]; present {
		t.Fatalf("inventory exposes retired identity: %#v", view)
	}
	for _, raw := range view["repositories"].([]any) {
		if asMap(t, raw)["dir"] != nil {
			t.Fatalf("inventory leaks host paths: %#v", raw)
		}
	}

	catalogPath := "/snapshot/v1"
	status, resolved, _ := httpSurfaceRequest(t, server, http.MethodPost, catalogPath+"/datasets:resolve", map[string]any{
		"dataset": "adhoc", "revision": 1,
		"sources": []map[string]any{{"repository": repositoryID, "selector": "refs/heads/main"}},
	}, principal)
	if status != http.StatusOK || asMap(t, resolved)["setId"] != "adhoc" {
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
	// The KC application composes its own routes with Snapshot Store's service.
	for _, dir := range []string{"../server", "../snapshot/service"} {
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			name := file.Name()
			if file.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			for _, match := range routeRegistration.FindAllStringSubmatch(string(raw), -1) {
				routes = append(routes, publicHTTPRoute{Method: match[1], Pattern: match[2]})
			}
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
	if strings.HasPrefix(pattern, "/api/v1/") {
		return strings.NewReplacer("{repository}", "native-repo", "{pull}", "one").Replace(pattern)
	}
	replacer := strings.NewReplacer(
		"{name}", "declarative-index",
		"{repository}", "kr:%2F%2Facme%2Frepository",
		"{dataset}", "agent",
		"{command}", "command-1",
		"{grant}", "grant-1",
		"{hook}", "hook-1",
		"{share}", "share-1",
		"{surface}", "consumer",
	)
	return replacer.Replace(pattern)
}

func allowedHTTPPath(path string) bool {
	for _, exact := range []string{"/health", "/livez", "/readyz", "/readyz/{surface}", "/metrics", "/ui", "/ui/"} {
		if path == exact {
			return true
		}
	}
	for _, namespace := range []string{
		"/api/v1/", "/identity/v1/", "/snapshot/v1/", "/object/v1/", "/snapshot/v1/files/",
		"/writer/v1/", "/admin/v1/", "/operations/v1/",
	} {
		if strings.HasPrefix(path, namespace) {
			return true
		}
	}
	return false
}

func TestRetiredHTTPRoutesAreNotFound(t *testing.T) {
	handler := serv.HTTPHandlerWithOptions(testkit.TempDir(t), serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	retired := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/object/v1/addresses:read"},
		{http.MethodPost, "/object/v1/schemas:page"},
		{http.MethodPost, "/object/v1/schemas:get"},
		{http.MethodPost, "/object/v1/log:get"},
		{http.MethodPost, "/object/v1/provenance:get"},
		{http.MethodPost, "/operations/v1/projections:notify"},
		{http.MethodPost, "/operations/v1/traces:get"},
		{http.MethodPost, "/writer/v1/repositories/" + url.PathEscape("kr://acme/repository") + "/proposals"},
		{http.MethodPost, "/admin/v1/grants/grant-1/remove"},
		{http.MethodPost, "/operations/v1/hooks/hook-1/remove"},
		{http.MethodPost, "/operations/v1/gates/gate-1/remove"},
		{http.MethodGet, "/operations/v1/gates"},
		{http.MethodPost, "/governance/v1/proposals"},
	}
	for _, route := range retired {
		request, err := http.NewRequest(route.method, server.URL+route.path, bytes.NewReader([]byte("{}")))
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
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			t.Errorf("%s %s still succeeds with %d", route.method, route.path, response.StatusCode)
		}
	}
}

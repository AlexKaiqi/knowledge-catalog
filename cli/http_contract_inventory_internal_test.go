package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type httpRouteEvidence struct {
	method string
	target string
	test   string
}

// These routes are intentionally not emitted by remote CLI dispatch. Their
// protocol semantics are exercised directly by the named HTTP/host tests.
var httpOnlyRouteEvidence = []httpRouteEvidence{
	{http.MethodGet, "/health", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/livez", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/readyz", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/readyz/consumer", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/metrics", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/identity/v1/whoami", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/catalog/v1/catalogs", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodPost, "/catalog/v1/catalogs/catalog-A/workspaces/resolve", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodPost, "/knowledge/v1/addresses:read", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodPost, "/workspace-files/v1/mounts:list", "TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange"},
	{http.MethodPost, "/workspace-files/v1/tree:list", "TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange"},
	{http.MethodPost, "/workspace-files/v1/file:read", "TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange"},
	{http.MethodPost, "/writer/v1/repositories/repo-A/proposals", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
}

var registeredRoutePattern = regexp.MustCompile(`mux\.HandleFunc\("((?:GET|POST|PUT|PATCH|DELETE) [^"]+)"`)

// TestEveryPublicHTTPRouteHasOwnedProtocolEvidence makes the production route
// registry the denominator. A new route must be owned either by a remote CLI
// transport/handler-DTO compatibility test or by a direct HTTP-only success
// scenario. Domain semantics stay in their application-level journeys.
func TestEveryPublicHTTPRouteHasOwnedProtocolEvidence(t *testing.T) {
	registered := productionHTTPRoutePatterns(t)
	if len(registered) != 56 {
		t.Fatalf("public HTTP route count changed from the reviewed 56 to %d; add protocol evidence for the new surface", len(registered))
	}
	if len(remoteDispatchRoutes) != 43 || len(httpOnlyRouteEvidence) != 13 {
		t.Fatalf("HTTP evidence partition changed: remote=%d HTTP-only=%d, want 43+13", len(remoteDispatchRoutes), len(httpOnlyRouteEvidence))
	}

	matcher := http.NewServeMux()
	for _, pattern := range registered {
		matcher.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	}
	owned := map[string][]string{}
	claim := func(method, target, owner string) {
		t.Helper()
		request := httptest.NewRequest(method, target, nil)
		response := httptest.NewRecorder()
		matcher.ServeHTTP(response, request)
		if request.Pattern == "" || response.Code == http.StatusNotFound || response.Code == http.StatusMethodNotAllowed {
			t.Fatalf("evidence %s does not match a production route: %s %s", owner, method, target)
		}
		owned[request.Pattern] = append(owned[request.Pattern], owner)
	}
	for _, route := range remoteDispatchRoutes {
		claim(route.method, route.target, "remote CLI: "+route.path)
	}
	for _, route := range httpOnlyRouteEvidence {
		claim(route.method, route.target, route.test)
	}

	for _, pattern := range registered {
		owners := owned[pattern]
		if len(owners) == 0 {
			t.Errorf("public HTTP route has no transport evidence owner: %s", pattern)
		} else if len(owners) > 1 {
			t.Errorf("public HTTP route has overlapping evidence owners: %s -> %v", pattern, owners)
		}
	}
	for pattern := range owned {
		if !containsString(registered, pattern) {
			t.Errorf("evidence owns a route absent from production: %s", pattern)
		}
	}
}

func productionHTTPRoutePatterns(t *testing.T) []string {
	t.Helper()
	var routes []string
	for _, name := range []string{"serve_routes.go", "service_routes.go", "service_management_routes.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range registeredRoutePattern.FindAllStringSubmatch(string(raw), -1) {
			routes = append(routes, match[1])
		}
	}
	sort.Strings(routes)
	return routes
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

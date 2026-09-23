package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"kc/httpsurface"
)

type httpRouteEvidence struct {
	method string
	target string
	test   string
}

// These routes are owned by named tests of the actual HTTP handler, host, or
// end-to-end Client -> Server journey, rather than the generic remote-dispatch
// matrix. Some are also available through CLI, whose successful production
// requests and responses are checked by their named journey owner.
var httpOnlyRouteEvidence = []httpRouteEvidence{
	{http.MethodGet, "/health", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/livez", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/readyz", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/readyz/consumer", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/metrics", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/identity/v1/whoami", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/identity/v1/auth", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodPost, "/identity/v1/token", "TestIdentityTokenBrokerKeepsApplicationSecretOnServer"},
	{http.MethodPost, "/identity/v1/authorize", "TestBrowserAuthorizationUsesFixedDeploymentUpstream"},
	{http.MethodPost, "/identity/v1/authorize:poll", "TestBrowserAuthorizationUsesFixedDeploymentUpstream"},
	{http.MethodGet, "/repositories/kr:%2F%2Fkaiqidong%2Fnotes", "TestRepositoryManagementPageLoadsWithoutExposingAuthority"},
	{http.MethodGet, "/assets/repository.js", "TestRepositoryManagementPageLoadsWithoutExposingAuthority"},
	{http.MethodGet, "/console", "TestConsolePageLoadsWithoutExposingAuthority"},
	{http.MethodGet, "/assets/console.js", "TestConsolePageLoadsWithoutExposingAuthority"},
	{http.MethodGet, "/ui/", "TestDatasetUILoadsBuildHintWithoutExposingAuthority"},
	{http.MethodGet, "/operations/v1/stores", "TestStoresObservationOmitsSecretsAndHomeLayout"},
	{http.MethodGet, "/catalog/v1/repositories", "TestManagedProductHumanSelfServiceOnLiveGitea"},
	{http.MethodGet, "/catalog/v1/catalogs/catalog-A/repositories", "TestCatalogViewsChecksAndKnowledgeResolve"},
	{http.MethodPost, "/catalog/v1/catalogs/catalog-A/repositories:create", "TestManagedRepositoryCreateUsesTypedClientWithoutCatalogDiscovery"},
	{http.MethodGet, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fnotes", "TestManagedProductHumanSelfServiceOnLiveGitea"},
	{http.MethodGet, "/identity/v1/admission", "TestAdmissionCLIReportsCurrentGrantsAndExternalRequestRoute"},
	{http.MethodGet, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fnotes/shares", "TestManagedProductHumanSelfServiceOnLiveGitea"},
	{http.MethodPost, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fnotes/shares", "TestManagedProductHumanSelfServiceOnLiveGitea"},
	{http.MethodDelete, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fnotes/shares/share-one", "TestManagedProductHumanSelfServiceOnLiveGitea"},
	{http.MethodPost, "/catalog/v1/catalogs/catalog-A/repositories:connect", "TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding"},
	{http.MethodGet, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fexisting/connection", "TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding"},
	{http.MethodPost, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fexisting/connection:check", "TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding"},
	{http.MethodPost, "/catalog/v1/repositories/kr:%2F%2Fkaiqidong%2Fexisting/connection:rotate", "TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding"},
	{http.MethodPost, "/catalog/v1/catalogs/catalog-A/repositories/repo-A/archive", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodGet, "/catalog/v1/catalogs/catalog-A/datasets", "TestCatalogViewsChecksAndKnowledgeResolve"},
	{http.MethodGet, "/catalog/v1/catalogs/catalog-A/datasets/agent", "TestCatalogViewsChecksAndKnowledgeResolve"},
	{http.MethodPost, "/catalog/v1/catalogs/catalog-A/datasets:resolve", "TestHTTPOnlyServiceRoutesReturnSuccessfulProtocolResponses"},
	{http.MethodPost, "/knowledge/v1/search:rerank", "TestHTTPSearchRerankPreservesRetrievalEvidenceAndUsesOneFixedView"},
	{http.MethodPost, "/knowledge/v1/rerank", "TestHTTPRerankReadsAuthorizedCanonicalCandidatesAndProjectsModelFields"},
	{http.MethodPost, "/operations/v1/retrieval-log:query", "TestTypedRetrievalEvidenceQueryAndTraining"},
	{http.MethodPost, "/operations/v1/retrieval-training:query", "TestTypedRetrievalEvidenceQueryAndTraining"},
	{http.MethodPost, "/operations/v1/refine-log:query", "TestRerankEvidenceFeedbackAndTrainingSampleJourney"},
	{http.MethodPost, "/operations/v1/rerank-training:query", "TestRerankEvidenceFeedbackAndTrainingSampleJourney"},
	{http.MethodPost, "/dataset-files/v1/mounts:list", "TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange"},
	{http.MethodPost, "/dataset-files/v1/tree:list", "TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange"},
	{http.MethodPost, "/dataset-files/v1/file:read", "TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange"},
}

var registeredRoutePattern = regexp.MustCompile(`mux\.HandleFunc\("((?:GET|POST|PUT|PATCH|DELETE) [^"]+)"`)

// TestEveryPublicHTTPRouteHasOwnedProtocolEvidence makes the production route
// registry the denominator. A new route must be owned either by a remote CLI
// transport/handler-DTO compatibility test or by a successful production
// HTTP/host journey. Domain semantics stay in their application-level journeys.
func TestEveryPublicHTTPRouteHasOwnedProtocolEvidence(t *testing.T) {
	registered := productionHTTPRoutePatterns(t)
	if len(registered) != 87 {
		t.Fatalf("public HTTP route count changed from the reviewed 86 to %d; add protocol evidence for the new surface", len(registered))
	}
	want := httpsurface.Patterns()
	if len(want) != len(registered) {
		t.Fatalf("HTTP registry package has %d patterns, production mux has %d", len(want), len(registered))
	}
	for i := range registered {
		if registered[i] != want[i] {
			t.Fatalf("HTTP registry drifted from production mux at %d: registry %q mux %q", i, want[i], registered[i])
		}
	}
	if len(remoteDispatchRoutes) != 47 || len(httpOnlyRouteEvidence) != 41 {
		t.Fatalf("HTTP evidence partition changed: remote=%d direct-journey=%d, want 47+40", len(remoteDispatchRoutes), len(httpOnlyRouteEvidence))
	}
	tests := httpEvidenceTestFunctions(t)

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
		if !tests[route.test] {
			t.Errorf("HTTP evidence owner is not an existing test function: %s", route.test)
		}
		claim(route.method, route.target, route.test)
	}

	for _, pattern := range registered {
		owners := owned[pattern]
		if len(owners) == 0 {
			t.Errorf("public HTTP route has no transport evidence owner: %s", pattern)
		} else if len(owners) > 1 && !sharedRemoteCLIOwners(owners) {
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
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range registeredRoutePattern.FindAllStringSubmatch(string(raw), -1) {
			if prior, duplicate := seen[match[1]]; duplicate {
				t.Fatalf("duplicate production HTTP registration %s in %s and %s", match[1], prior, name)
			}
			seen[match[1]] = name
			routes = append(routes, match[1])
		}
	}
	sort.Strings(routes)
	return routes
}

func httpEvidenceTestFunctions(t *testing.T) map[string]bool {
	t.Helper()
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.Name(), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				names[fn.Name.Name] = true
			}
		}
	}
	return names
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

// sharedRemoteCLIOwners allows two public argv paths to dispatch to one HTTP
// route (access / invoke → resources:access). HTTP-only evidence
// must still be exclusive with remote CLI.
func sharedRemoteCLIOwners(owners []string) bool {
	if len(owners) < 2 {
		return false
	}
	for _, owner := range owners {
		if !strings.HasPrefix(owner, "remote CLI: ") {
			return false
		}
	}
	return true
}

package cli_test

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

// This is the public-surface counterpart of the provider contract test: the
// runtime and OpenSearch are separate Docker services, while all setup,
// refresh, and SEARCH operations cross the kc HTTP facade.
func TestLiveHTTPDynamicStateSearchJourney(t *testing.T) {
	runtimeURL := strings.TrimSpace(os.Getenv("KC_TEST_STATE_RUNTIME_URL"))
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if runtimeURL == "" || opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_STATE_RUNTIME_URL and KC_TEST_OPENSEARCH_URL are required")
		}
		t.Skip("run make test-state-runtime-e2e")
	}
	home := testkit.TempDir(t)
	repositoryID := "kr://docker/public/services"
	body(t, kc(home, "init", "--catalog", "kr://docker/catalog"))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	body(t, kc(home, "repo-add", "--repo", repositoryID))
	body(t, kc(home, "put", "--command-id", "state-schema", "--repo", repositoryID,
		"--object", "schema/service.health",
		"--value", `{"entity":"Service","aspect":"health","fields":{"status":{"type":"string","access":["text","filter"]}}}`))
	body(t, kc(home, "put", "--command-id", "state-binding", "--repo", repositoryID,
		"--object", "Service:orders", "--aspect", "health", "--schema-ref", "schema/service.health", "--value", "null",
		"--value-source", `{"kind":"binding","binding":{"mode":"state","runtime":"health","protocol":"resource-access/v1","operations":{"lookup":{"call":"health.lookup"}}}}`))
	body(t, kc(home, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", repositoryID+"=refs/heads/main@"))
	before := asMap(t, body(t, kc(home, "resolve", "--repo", repositoryID, "--object", "Service:orders", "--aspect", "health", "--ref", "refs/heads/main")))["commit"]
	expectCode(t, kc(home, "search", "--workspace", "agent", "--query", "healthy"), "CAPABILITY_UNSATISFIED")

	lookup, err := cli.NewHTTPStateLookup(runtimeURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{StateLookup: lookup})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	sync := asMap(t, postAny(t, server.URL, "index-sync", map[string]any{
		"repo": repositoryID, "ref": "refs/heads/main", "request-id": "state-notice-1",
	}))
	if asMap(t, sync["state"])["revision"] == "" {
		t.Fatalf("index-sync did not publish State projection: %#v", sync)
	}
	search := asMap(t, postAny(t, server.URL, "search", map[string]any{
		"workspace": "agent", "query": "healthy", "request-id": "state-search-1",
	}))
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("dynamic field did not discover candidate: %#v", search)
	}
	hit := asMap(t, hits[0])
	if asMap(t, asMap(t, hit["knowledge"])["value"])["health"] == nil {
		t.Fatalf("dynamic hit was not hydrated: %#v", hit)
	}
	if len(asMap(t, hit["version"])["observations"].([]any)) != 1 {
		t.Fatalf("dynamic hit lost observation basis: %#v", hit)
	}
	view := asMap(t, search["searchView"])
	if asMap(t, view["projectionRevisions"])[repositoryID] == "" {
		t.Fatalf("SearchView did not bind provider revision: %#v", view)
	}
	after := asMap(t, body(t, kc(home, "resolve", "--repo", repositoryID, "--object", "Service:orders", "--aspect", "health", "--ref", "refs/heads/main")))["commit"]
	if after != before {
		t.Fatalf("observation refresh moved Repository HEAD: before=%v after=%v", before, after)
	}
}

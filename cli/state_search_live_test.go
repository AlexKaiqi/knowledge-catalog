package cli_test

import (
	"encoding/json"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	serv "kc/server"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// This is the public-surface counterpart of the provider contract test: the
// runtime and OpenSearch are separate Docker services. Authored declarations
// are selected at publication; refresh and SEARCH cross the typed HTTP facade.
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
	body(t, kc(home, "local", "init"))
	body(t, kc(home, "local", "store", "set", "--index", "opensearch"))
	body(t, kc(home, "local", "store", "set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repositoryID)
	body(t, kc(home, "writer", "put", testObjectReference(t, repositoryID, "schema/service.health", ""), "--repo", repositoryID, "--command-id", "state-schema", "--value", `{"entity":"Service","aspect":"health","fields":{"status":{"type":"string"}}}`))
	body(t, kc(home, "writer", "put", testObjectReference(t, repositoryID, "service/orders", "/properties"), "--repo", repositoryID, "--command-id", "state-entity", "--value", `{"name":"orders"}`))
	descriptorID := putStateBinding(t, home, repositoryID, "service/orders", "health", "schema/service.health", runtimeURL)
	descriptor := bindingfixture.Descriptor(runtimeURL)
	descriptor["identity"] = "shared"
	rawDescriptor, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	body(t, kc(home, "writer", "put", "urn:kc:object:"+descriptorID, "--repo", repositoryID, "--command-id", "shared-state-descriptor", "--value", string(rawDescriptor)))
	seedTextDeclaration(t, home, repositoryID, "index/service-health", "schema/service.health", "status")
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repositoryID))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "file.read", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:maintainer", "--action", "dataset.manage", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:maintainer", "--action", "index.manage", "--dataset", "agent"))
	before := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))["commit"]

	lookup := serv.NewHTTPStateLookup(nil)
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{StateLookup: lookup})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	sync := asMap(t, postAs(t, server.URL, "/operations/v1/indexes:prepare", "agent:maintainer", map[string]any{
		"dataset": "agent", "revision": 1,
	}))
	if sync["dataset"] != "agent" || sync["revision"] != float64(1) || datasetIndexObservation(t, sync, repositoryID)["revision"] == "" {
		t.Fatalf("index prepare did not publish State projection: %#v", sync)
	}
	search := asMap(t, postAs(t, server.URL, "/object/v1/search", "agent:http-test", map[string]any{
		"dataset": "agent", "revision": 1, "query": "healthy",
	}))
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("dynamic field did not discover candidate: %#v", search)
	}
	hit := asMap(t, hits[0])
	if asMap(t, asMap(t, hit["object"])["value"])["health"] == nil {
		t.Fatalf("dynamic hit was not hydrated: %#v", hit)
	}
	if len(asMap(t, hit["version"])["observations"].([]any)) != 1 {
		t.Fatalf("dynamic hit lost observation basis: %#v", hit)
	}
	view := asMap(t, search["searchView"])
	if asMap(t, view["snapshots"])[repositoryID] != before || asMap(t, view["indexRevisions"])[repositoryID] != datasetIndexObservation(t, sync, repositoryID)["revision"] {
		t.Fatalf("SearchView did not bind provider revision: %#v", view)
	}
	// State is an external observation, not a Snapshot health unit. Check the
	// unchanged Snapshot authority directly, just as before the refresh.
	after := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))["commit"]
	if after != before {
		t.Fatalf("observation refresh moved Repository HEAD: before=%v after=%v", before, after)
	}
}

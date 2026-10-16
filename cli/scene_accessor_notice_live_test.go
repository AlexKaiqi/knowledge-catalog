package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Scene accessor containers serve Bound State. An explicit Dataset refresh must
// replace its observation when source.json changes, without moving source HEAD.
func TestSceneAccessorRefreshUpdatesSearchIndex(t *testing.T) {
	runtimeURL := strings.TrimSpace(os.Getenv("KC_TEST_STATE_RUNTIME_URL"))
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	sourcePath := strings.TrimSpace(os.Getenv("KC_TEST_STATE_SOURCE_PATH"))
	if runtimeURL == "" || opensearchURL == "" || sourcePath == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_STATE_RUNTIME_URL, KC_TEST_OPENSEARCH_URL, and KC_TEST_STATE_SOURCE_PATH are required")
		}
		t.Skip("run scene accessor compose and set KC_TEST_STATE_SOURCE_PATH")
	}
	original, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(sourcePath, original, 0o644) })

	home := testkit.TempDir(t)
	repositoryID := "kr://scene/accessor-index"
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
	body(t, kc(home, "writer", "put", "urn:kc:object:"+descriptorID, "--repo", repositoryID, "--command-id", "state-shared-descriptor", "--value", string(rawDescriptor)))
	seedTextDeclaration(t, home, repositoryID, "index/service-health", "schema/service.health", "status")
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repositoryID))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "file.read", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:observer", "--action", "dataset.manage", "--dataset", "agent"))
	before := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))["commit"]

	lookup := serv.NewHTTPStateLookup(nil)
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{StateLookup: lookup})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	stateOf := func(payload any) map[string]any {
		t.Helper()
		receipt := asMap(t, payload)
		members, ok := receipt["members"].([]any)
		if receipt["dataset"] != "agent" || receipt["revision"] != float64(1) || !ok || len(members) != 1 || asMap(t, members[0])["repository"] != repositoryID || asMap(t, members[0])["commit"] != before {
			t.Fatalf("refresh lost its accepted Dataset revision or source vector: %#v", receipt)
		}
		state := datasetProjectionState(t, receipt, repositoryID)
		revision, _ := state["revision"].(string)
		digest, _ := state["observationDigest"].(string)
		if state["mode"] != "rebuild" || revision == "" || digest == "" {
			t.Fatalf("refresh omitted its published observation facts: %#v", state)
		}
		return state
	}
	refresh := func() map[string]any {
		return stateOf(postAs(t, server.URL, "/operations/v1/projections:sync", "agent:observer", map[string]any{
			"dataset": "agent", "revision": 1,
		}))
	}
	first := refresh()
	healthy := asMap(t, postAs(t, server.URL, "/object/v1/search", "agent:http-test", map[string]any{
		"dataset": "agent", "revision": 1, "query": "healthy",
	}))
	if hits := healthy["hits"].([]any); len(hits) != 1 {
		t.Fatalf("initial State index missed healthy: %#v", healthy)
	}

	if err := os.WriteFile(sourcePath, []byte(`{
  "revision": "docker-health-2",
  "generation": "docker-runtime-v1",
  "entities": {
    "service/orders": {
      "status": "degraded",
      "runtime": "docker"
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	second := refresh()
	if second["revision"] == first["revision"] || second["observationDigest"] == first["observationDigest"] {
		t.Fatalf("explicit refresh must publish a new observation on the same Dataset revision: first=%#v second=%#v", first, second)
	}

	degraded := asMap(t, postAs(t, server.URL, "/object/v1/search", "agent:http-test", map[string]any{
		"dataset": "agent", "revision": 1, "query": "degraded",
	}))
	if hits := degraded["hits"].([]any); len(hits) != 1 {
		t.Fatalf("index did not pick up degraded after refresh: %#v", degraded)
	}
	stale := asMap(t, postAs(t, server.URL, "/object/v1/search", "agent:http-test", map[string]any{
		"dataset": "agent", "revision": 1, "query": "healthy",
	}))
	if hits := stale["hits"].([]any); len(hits) != 0 {
		t.Fatalf("old healthy hit survived index refresh: %#v", stale)
	}
	after := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))["commit"]
	if after != before {
		t.Fatalf("Dataset refresh moved Repository HEAD: before=%v after=%v", before, after)
	}
}

func postAs(t *testing.T, base, path, principal string, body map[string]any) any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kc-As", principal)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s as %s status=%d payload=%s", http.MethodPost, path, principal, resp.StatusCode, payload)
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err, string(payload))
	}
	return decoded
}

package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

// Scene accessor containers serve Bound State. Observer notice must republish
// the OpenSearch State projection when source.json changes, without moving HEAD.
func TestSceneAccessorNoticeUpdatesSearchIndex(t *testing.T) {
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
	body(t, kc(home, "init", "--catalog", "kr://scene/catalog"))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repositoryID)
	body(t, kc(home, "put", "--command-id", "state-schema", "--repo", repositoryID,
		"--object", "schema/service.health",
		"--value", `{"entity":"Service","aspect":"health","origin":"`+runtimeURL+`","fields":{"status":{"type":"string","access":["text","filter"]}}}`))
	body(t, kc(home, "put", "--command-id", "state-entity", "--repo", repositoryID,
		"--object", "Service:orders", "--aspect", "properties", "--value", `{"name":"orders"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repositoryID+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read-workspace", "--catalog", "kr://scene/catalog", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read,search", "--repo", repositoryID))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "projection.manage", "--repo", repositoryID))
	body(t, kc(home, "allow", "--principal", "agent:observer", "--action", "projection.manage", "--repo", repositoryID))
	before := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))["commit"]

	lookup := cli.NewHTTPStateLookup(nil)
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{StateLookup: lookup})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	first := asMap(t, postAny(t, server.URL, "/operations/v1/projections:notice", map[string]any{
		"repository":     repositoryID,
		"address":        map[string]any{"kind": "Aspect", "objectId": "Service:orders", "aspectName": "health"},
		"sourceRevision": "docker-health-1",
	}))
	if first["revision"] == "" || first["basisCommit"] != before {
		t.Fatalf("first notice did not publish State at HEAD: %#v want %v", first, before)
	}
	healthy := asMap(t, postAny(t, server.URL, "/knowledge/v1/search", map[string]any{
		"dataset": "agent", "query": "healthy",
	}))
	if hits := healthy["hits"].([]any); len(hits) != 1 {
		t.Fatalf("initial State index missed healthy: %#v", healthy)
	}

	if err := os.WriteFile(sourcePath, []byte(`{
  "revision": "docker-health-2",
  "generation": "docker-runtime-v1",
  "entities": {
    "Service:orders": {
      "status": "degraded",
      "runtime": "docker"
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	second := asMap(t, postAs(t, server.URL, "/operations/v1/projections:notice", "agent:observer", map[string]any{
		"repository":     repositoryID,
		"address":        map[string]any{"kind": "Aspect", "objectId": "Service:orders", "aspectName": "health"},
		"sourceRevision": "docker-health-2",
	}))
	if second["revision"] == "" || second["revision"] == first["revision"] || second["basisCommit"] != before {
		t.Fatalf("observer notice must publish a new index revision on the same commit: first=%#v second=%#v", first, second)
	}

	degraded := asMap(t, postAny(t, server.URL, "/knowledge/v1/search", map[string]any{
		"dataset": "agent", "query": "degraded",
	}))
	if hits := degraded["hits"].([]any); len(hits) != 1 {
		t.Fatalf("index did not pick up degraded after notice: %#v", degraded)
	}
	stale := asMap(t, postAny(t, server.URL, "/knowledge/v1/search", map[string]any{
		"dataset": "agent", "query": "healthy",
	}))
	if hits := stale["hits"].([]any); len(hits) != 0 {
		t.Fatalf("old healthy hit survived index refresh: %#v", stale)
	}
	after := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))["commit"]
	if after != before {
		t.Fatalf("notice moved Repository HEAD: before=%v after=%v", before, after)
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

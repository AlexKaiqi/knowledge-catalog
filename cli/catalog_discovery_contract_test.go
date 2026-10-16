//go:build catalog_discovery_contract

package cli_test

import (
	"bytes"
	"encoding/json"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
)

// Real deployed media: inventory discovery never substitutes for Dataset
// consumption, while a selected Dataset exposes its complete frozen scope.
func TestInventoryDiscoveryRequiresDatasetConsumptionBeforeSearch(t *testing.T) {
	endpoint := os.Getenv("KC_TEST_OPENSEARCH_URL")
	if endpoint == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required for this explicit contract")
	}
	t.Setenv("KC_REQUIRE_LIVE_ADAPTERS", "1")
	t.Setenv("KC_DATASET", "")
	t.Setenv("KC_CATALOG", "")
	live := testkit.NewLakeFSLive(t, os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"), os.Getenv("KC_LAKEFS_CREDENTIAL"), os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE"))
	cfg, config := declaredDeployment(t, false)
	cfg.Stores.Index = "opensearch"
	cfg.Stores.OpenSearch.URL = endpoint
	cfg.Stores.OpenSearch.PrimaryShards = 1
	repos := []string{live.NewRepo(), live.NewRepo(), live.NewRepo()}
	for _, id := range repos {
		cfg.Repositories = append(cfg.Repositories, apphome.RepositoryBinding{ID: id, Driver: "lakefs", DSN: live.DSN(id)})
	}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", config))
	handler, err := serv.HTTPHandlerFromConfig(config, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	defer handler.(interface{ Close() error }).Close()
	admin := func(args ...string) kcRunResult { return kcRemote(t, server.URL, cfg.BootstrapPrincipal, args...) }
	viewer := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "viewer", args...) }
	for i, repo := range repos {
		name := strconv.Itoa(i)
		schema := "schema/note-" + name
		body(t, admin("attach", "--repo", repo))
		body(t, admin("writer", "put", "urn:kc:object:"+schema, "--repo", repo, "--command-id", "schema-"+name, "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string"}}}`))
		content := []string{"discovery phrase confidentialalpha", "discovery phrase confidentialbeta", "discovery phrase excludedbody"}[i]
		raw, _ := json.Marshal(map[string]string{"body": content})
		body(t, admin("writer", "put", "urn:kc:object:note/"+name, "--repo", repo, "--command-id", "note-"+name, "--schema-ref", schema, "--value", string(raw)))
		declaration := `{"catalog":"declarative-index/opensearch@2026-10","inputs":[{"from":"` + schema + `#/fields/body"}],"build":{"kind":"text"},"uses":["text"]}`
		body(t, admin("writer", "put", "urn:kc:object:index/note-"+name+"#/index/body", "--repo", repo, "--command-id", "index-"+name, "--kind", "Member", "--schema-ref", "schema/core/index-declaration/v1", "--value", declaration))
	}
	body(t, admin("dataset", "define", "--dataset", "published", "--revision", "1", "--source", repos[0], "--source", repos[1]))
	body(t, admin("dataset", "define", "--dataset", "private", "--revision", "1", "--source", repos[2]))
	body(t, admin("grant", "add", "--principal", "viewer", "--action", "inventory.read"))
	show := asMap(t, body(t, viewer("show")))
	if len(show["datasets"].([]any)) != 0 {
		t.Fatalf("inventory revealed unconsumable Dataset: %#v", show)
	}
	expectCode(t, viewer("search", "--dataset", "published", "--revision", "1", "--query", "discovery phrase"), "FORBIDDEN")
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "viewer", "--dataset", "published", "--action", "file.read")))
	post := func(principal, path string, payload any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", server.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Kc-As", principal)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, out
	}
	status, result := post("viewer", "/object/v1/search", map[string]any{"dataset": "published", "revision": 1, "query": "discovery phrase"})
	if status != http.StatusOK || result["completeness"] != "complete" || len(result["hits"].([]any)) != 2 {
		t.Fatalf("Dataset search lost complete selected scope: %d %#v", status, result)
	}
	snapshots := asMap(t, asMap(t, result["searchView"])["snapshots"])
	if len(snapshots) != 2 || snapshots[repos[0]] == nil || snapshots[repos[1]] == nil || snapshots[repos[2]] != nil {
		t.Fatalf("scope/basis drift: %#v", result)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "confidentialalpha") || !strings.Contains(string(raw), "confidentialbeta") || strings.Contains(string(raw), "excludedbody") {
		t.Fatalf("Dataset consumption returned the wrong bodies: %s", raw)
	}
	expectCode(t, viewer("read", "urn:kc:object:note/0", "--repo", repos[0]), "FORBIDDEN")
	expectCode(t, viewer("search", "--dataset", "private", "--query", "discovery phrase"), "FORBIDDEN")
	status, out := post("viewer", "/object/v1/search", map[string]any{"dataset": "published", "inventoryDiscovery": true, "query": "discovery phrase"})
	if status != http.StatusBadRequest || asMap(t, out["error"])["code"] != "USAGE_INVALID" {
		t.Fatalf("retired inventory bypass accepted: %d %#v", status, out)
	}
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, viewer("search", "--dataset", "published", "--revision", "1", "--query", "discovery phrase"), "FORBIDDEN")
	expectCode(t, kcRemote(t, server.URL, "stranger", "search", "--dataset", "published", "--query", "discovery phrase"), "FORBIDDEN")
}

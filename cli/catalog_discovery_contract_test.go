//go:build catalog_discovery_contract

package cli_test

import (
	"bytes"
	"encoding/json"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
)

// Explicit acceptance: real Catalog Snapshot, lakeFS Snapshot, OpenSearch and typed
// Client/Server. No read/search/consume grant is given to the discovery viewer.
func TestInventoryDiscoveryActualServerPinsSelectedSourcesAndMasksBodies(t *testing.T) {
	endpoint := os.Getenv("KC_TEST_OPENSEARCH_URL")
	if endpoint == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required for this explicit contract")
	}
	t.Setenv("KC_REQUIRE_LIVE_ADAPTERS", "1")
	t.Setenv("KC_DATASET", "")
	t.Setenv("KC_CATALOG", "")
	live := testkit.NewLakeFSLive(t, os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"), os.Getenv("KC_LAKEFS_CREDENTIAL"), os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE"))
	cfg, config := declaredDeployment(t, false)
	cfg.Registry.DiscoverySetID = "published"
	cfg.Stores.Index = "opensearch"
	cfg.Stores.OpenSearch.URL = endpoint
	cfg.Stores.OpenSearch.PrimaryShards = 1
	repos := []string{live.NewRepo(), live.NewRepo(), live.NewRepo()}
	for _, id := range repos {
		cfg.Repositories = append(cfg.Repositories, apphome.RepositoryBinding{ID: id, Driver: "lakefs", DSN: live.DSN(id)})
	}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
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
		body(t, admin("attach", "--repo", repo))
		body(t, admin("writer", "put", testObjectReference(t, repo, "schema/note", ""), "--repo", repo, "--command-id", "schema-"+repo, "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
		content := []string{"discovery phrase confidentialalpha", "discovery phrase confidentialbeta", "discovery phrase excludedbody"}[i]
		raw, _ := json.Marshal(map[string]string{"body": content})
		body(t, admin("writer", "put", testObjectReference(t, repo, "note/one", ""), "--repo", repo, "--command-id", "note-"+repo, "--schema-ref", "schema/note", "--value", string(raw)))
		body(t, admin("operations", "projection", "sync", "--repo", repo))
	}
	body(t, admin("dataset", "define", "--dataset", "published", "--revision", "1", "--source", repos[0], "--source", repos[1]))
	body(t, admin("dataset", "define", "--dataset", "private", "--revision", "1", "--source", repos[2]))
	body(t, admin("grant", "add", "--principal", "viewer", "--action", "inventory.read"))
	show := asMap(t, body(t, viewer("show")))
	if show["discoveryWorkspaceId"] != "published" {
		t.Fatalf("missing configured entry %#v", show)
	}
	// Discovery SEARCH is HTTP-only: the CLI no longer carries a third search
	// scope, but the typed body keeps the inventoryDiscovery contract.
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
	// Discovery SEARCH requires the configured Workspace and its fixed pin; the
	// Server only trusts the typed inventoryDiscovery marker after matching them.
	status, pin := post("viewer", "/snapshot/v1/datasets/published/resolve", map[string]any{"inventoryDiscovery": true})
	if status != 200 {
		t.Fatalf("coordinate resolve required body grants %d %#v", status, pin)
	}
	search := func() map[string]any {
		status, out := post("viewer", "/object/v1/search", map[string]any{
			"dataset": "published", "pin": pin, "inventoryDiscovery": true, "query": "discovery phrase",
		})
		if status != http.StatusOK {
			t.Fatalf("discovery search status %d %#v", status, out)
		}
		return out
	}
	result := search()
	checkSelection := func(result map[string]any) {
		t.Helper()
		hits := result["hits"].([]any)
		if len(hits) != 2 {
			t.Fatalf("discovery candidate coverage %#v", result)
		}
		snapshots := asMap(t, asMap(t, result["searchView"])["snapshots"])
		if len(snapshots) != 2 || snapshots[repos[0]] == nil || snapshots[repos[1]] == nil || snapshots[repos[2]] != nil {
			t.Fatalf("selection/basis drift %#v", snapshots)
		}
		if result["completeness"] != "complete" {
			t.Fatalf("missing read implied partial %#v", result)
		}
	}
	checkSelection(result)
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "confidential") || strings.Contains(string(raw), "excludedbody") {
		t.Fatalf("discovery leaked Canonical body %s", raw)
	}
	readGrant := asMap(t, body(t, admin("grant", "add", "--principal", "viewer", "--repo", repos[0], "--action", "object.read")))
	result = search()
	checkSelection(result)
	raw, _ = json.Marshal(result)
	if !strings.Contains(string(raw), "confidentialalpha") || strings.Contains(string(raw), "confidentialbeta") {
		t.Fatalf("read grants did not control delivery %s", raw)
	}
	body(t, admin("grant", "remove", "--id", readGrant["id"].(string)))
	result = search()
	raw, _ = json.Marshal(result)
	if strings.Contains(string(raw), "confidential") {
		t.Fatal("revocation did not update delivery")
	}
	for _, tc := range []struct {
		workspace  string
		pin        any
		definition any
		code       string
	}{
		{"private", pin, nil, "FORBIDDEN"}, {"published", nil, nil, "USAGE_INVALID"}, {"published", pin, map[string]any{"revision": 1, "sources": []any{}}, "USAGE_INVALID"},
	} {
		input := map[string]any{"dataset": tc.workspace, "pin": tc.pin, "inventoryDiscovery": true, "query": "discovery phrase"}
		if tc.definition != nil {
			input["definition"] = tc.definition
		}
		status, out := post("viewer", "/object/v1/search", input)
		if status < 400 || asMap(t, out["error"])["code"] != tc.code {
			t.Fatalf("discovery scope bypass %d %#v", status, out)
		}
	}
	// CATALOG-02: an authenticated principal discovers a public Catalog
	// without a grant; discovery delivery stays masked (no bodies) and is
	// pinned to the configured discovery Workspace.
	status, out := post("stranger", "/object/v1/search", map[string]any{
		"dataset": "published", "pin": pin, "inventoryDiscovery": true, "query": "discovery phrase",
	})
	if status != http.StatusOK || out["completeness"] != "complete" {
		t.Fatalf("authenticated discovery of a public Catalog failed: %d %#v", status, out)
	}
	raw, _ = json.Marshal(out)
	if strings.Contains(string(raw), "confidential") || strings.Contains(string(raw), "excludedbody") {
		t.Fatalf("stranger discovery leaked Canonical body %s", raw)
	}
	t.Log("PASS selected discovery Workspace -> fixed SEARCH; inventory.read only; current body grants and scope boundaries")
}

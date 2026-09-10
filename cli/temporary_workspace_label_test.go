package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/snapshot"
)

func TestHTTPTemporaryWorkspaceLabelCannotImpersonatePublishedWorkspace(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	home := testkit.TempDir(t)
	catalogID := "kr://acme/temporary-label/catalog"
	repositoryID := "kr://acme/temporary-label/source"
	const label = "trusted"
	const namedReader = "agent:named-reader"
	const memberReader = "agent:member-reader"
	const namedSearcher = "agent:named-searcher"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))
	body(t, kc(home, "attach", "--repo", repositoryID))
	body(t, kc(home, "writer", "put", "--command-id", "temporary-label-seed", "--repo", repositoryID,
		"--object", "Policy:label", "--value", `{"body":"fixed label content"}`))
	body(t, kc(home, "workspace", "define", "--workspace", label, "--revision", "1", "--source", repositoryID))
	body(t, kc(home, "grant", "add", "--principal", namedReader,
		"--action", "workspace.resolve,workspace.consume", "--catalog", catalogID, "--workspace", label))
	body(t, kc(home, "grant", "add", "--principal", namedReader,
		"--action", "knowledge.read", "--repo", repositoryID))
	body(t, kc(home, "grant", "add", "--principal", memberReader,
		"--action", "workspace.resolve,workspace.consume,knowledge.read", "--repo", repositoryID))
	body(t, kc(home, "grant", "add", "--principal", namedSearcher,
		"--action", "workspace.consume", "--catalog", catalogID))
	body(t, kc(home, "grant", "add", "--principal", namedSearcher,
		"--action", "knowledge.search", "--catalog", catalogID, "--workspace", label))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	catalogPath := "/catalog/v1/catalogs/" + url.PathEscape(catalogID)
	status, namedPin, _ := httpSurfaceRequest(t, server, http.MethodPost, catalogPath+"/workspaces/"+label+"/resolve", map[string]any{}, namedReader)
	if status != http.StatusOK || asMap(t, namedPin)["workspaceId"] != label {
		t.Fatalf("published Workspace grant must still resolve its published recipe: %d %#v", status, namedPin)
	}
	sources := []map[string]any{{"repository": repositoryID, "selector": snapshot.DefaultRef}}
	request := map[string]any{"workspace": label, "revision": 1, "sources": sources}
	status, rejected, _ := httpSurfaceRequest(t, server, http.MethodPost, catalogPath+"/workspaces:resolve", request, namedReader)
	if status != http.StatusForbidden {
		t.Fatalf("caller label must not borrow a published Workspace resolve grant: %d %#v", status, rejected)
	}
	status, temporaryPin, _ := httpSurfaceRequest(t, server, http.MethodPost, catalogPath+"/workspaces:resolve", request, memberReader)
	if status != http.StatusOK || asMap(t, temporaryPin)["workspaceId"] != label {
		t.Fatalf("member grants must resolve a labeled temporary definition: %d %#v", status, temporaryPin)
	}
	definition := map[string]any{"workspaceId": label, "revision": 1, "sources": sources}
	pinWithDefinition := cloneJSONMap(t, asMap(t, temporaryPin))
	pinWithDefinition["catalog"] = catalogID
	pinWithDefinition["definition"] = definition
	read := map[string]any{"pin": pinWithDefinition, "object": "Policy:label"}
	status, result, _ := httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/objects:read", read, memberReader)
	if status != http.StatusOK {
		t.Fatalf("fixed temporary pin must replay with its original label: %d %#v", status, result)
	}
	values, ok := result.([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("temporary read omitted the seeded content: %#v", result)
	}
	value := asMap(t, values[0])
	if asMap(t, value["value"])["body"] != "fixed label content" || value["commit"] != asMap(t, asMap(t, temporaryPin)["repositories"])[repositoryID] {
		t.Fatalf("temporary read changed content or fixed authority version: %#v", value)
	}
	saved := cloneJSONMap(t, asMap(t, temporaryPin))
	saved["definition"], saved["catalog"] = definition, catalogID
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	delete(saved, "definition")
	delete(saved, "catalog")
	replayed := cli.Run([]string{"--server", server.URL, "--as", memberReader, "knowledge", "read", "--pin", string(raw), "--object", "Policy:label"})
	if replayed.Status != 0 {
		t.Fatalf("CLI could not replay a labeled temporary pin through the actual Server: %s", replayed.Stdout)
	}
	status, rejected, _ = httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/objects:read", read, namedReader)
	if status != http.StatusForbidden {
		t.Fatalf("temporary pin must not borrow published Workspace consume: %d %#v", status, rejected)
	}
	status, rejected, _ = httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/objects:read", map[string]any{
		"workspace": label, "definition": definition, "pin": temporaryPin, "object": "Policy:label",
	}, memberReader)
	if status != http.StatusBadRequest {
		t.Fatalf("explicit named selector mixed with temporary definition was accepted: %d %#v", status, rejected)
	}
	// A separately supplied named selector cannot turn a caller-owned definition
	// into published authority, including at the knowledge verb admission gate.
	search := map[string]any{"catalog": catalogID, "workspace": label, "definition": definition, "pin": temporaryPin, "query": "label"}
	status, rejected, _ = httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/search", search, namedSearcher)
	if status != http.StatusForbidden {
		t.Fatalf("temporary definition borrowed a named knowledge.search grant: %d %#v", status, rejected)
	}
	search["catalogDiscovery"] = true
	status, rejected, _ = httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/search", search, namedSearcher)
	if status != http.StatusBadRequest {
		t.Fatalf("temporary label forged Catalog discovery context: %d %#v", status, rejected)
	}
}

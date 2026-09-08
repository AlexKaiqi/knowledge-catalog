package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apphome "kc/home"
	"kc/kernel"
)

func TestCatalogDiscoveryClientResolvesConfiguredWorkspaceBeforeSearch(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_HOME", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_WORKSPACE", "ambient")
	t.Setenv("KC_CATALOG", "")
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if r.Method == "POST" {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.URL.Path {
		case "/catalog/v1/catalogs/kr://acme/catalog":
			_, _ = w.Write([]byte(`{"catalogId":"kr://acme/catalog","discoveryWorkspaceId":"published"}`))
		case "/catalog/v1/catalogs/kr://acme/catalog/workspaces/published/resolve":
			if body["catalogDiscovery"] != true {
				t.Error("missing discovery resolve context")
			}
			_, _ = w.Write([]byte(`{"workspaceId":"published","revision":1,"pinId":"fixed-pin","repositories":{"kr://acme/selected":"fixed"}}`))
		case "/knowledge/v1/search":
			if body["workspace"] != "published" || body["catalogDiscovery"] != true || body["pin"] == nil {
				t.Errorf("search lost explicit fixed discovery context %#v", body)
			}
			_, _ = w.Write([]byte(`{"hits":[],"completeness":"COMPLETE"}`))
		default:
			t.Errorf("unexpected discovery request %s", r.URL.Path)
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	result := Run([]string{"knowledge", "search", "--catalog", "kr://acme/catalog", "--query", "note", "--server", server.URL, "--as", "viewer"})
	if result.Status != 0 || len(paths) != 3 {
		t.Fatalf("catalog discovery did not resolve then search %#v %s", paths, result.Stdout)
	}
}

func TestCatalogDiscoveryContextRequiresExactPublishedWorkspaceAndAction(t *testing.T) {
	ws := &Home{Deployment: &apphome.DeploymentConfig{Catalogs: []apphome.CatalogBinding{{ID: "kr://acme/catalog", DiscoveryWorkspaceID: "published"}}}}
	flags := map[string]FlagValue{"catalog": "kr://acme/catalog", "workspace": "published", "pin": "{}", catalogDiscoveryFlag: true}
	if err := prepareCatalogDiscovery(ws, "knowledge.search", flags); err != nil {
		t.Fatal(err)
	}
	if !isCatalogDiscovery(flags) {
		t.Fatal("validated context not established")
	}
	for _, action := range []string{"knowledge.read", "knowledge.rerank", "file.read", "workspace.manage"} {
		if err := prepareCatalogDiscovery(ws, action, flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("discovery context bypassed %s: %v", action, err)
		}
	}
	flags["workspace"] = "private"
	if err := prepareCatalogDiscovery(ws, "knowledge.search", flags); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("unconfigured Workspace accepted %v", err)
	}
	flags["workspace"] = "published"
	delete(flags, "pin")
	if err := prepareCatalogDiscovery(ws, "knowledge.search", flags); err == nil || !strings.Contains(err.Error(), "pin") {
		t.Fatalf("search without fixed pin accepted %v", err)
	}
}

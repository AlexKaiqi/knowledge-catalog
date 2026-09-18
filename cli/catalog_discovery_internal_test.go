package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apphome "kc/home"
	"kc/kernel"
)

func TestKnowledgeSearchRejectsCatalogDiscoveryOperand(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("search --catalog must not reach HTTP")
	}))
	defer server.Close()
	result := Run([]string{"search", "--catalog", "kr://acme/catalog", "--query", "note", "--server", server.URL, "--as", "viewer"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
		t.Fatalf("catalog discovery operand must be rejected: %s", result.Stdout)
	}
}

func TestCatalogDiscoveryContextRequiresExactPublishedWorkspaceAndAction(t *testing.T) {
	ws := &Home{Deployment: &apphome.DeploymentConfig{Catalogs: []apphome.CatalogBinding{{ID: "kr://acme/catalog", DiscoverySetID: "published"}}}}
	flags := map[string]FlagValue{"catalog": "kr://acme/catalog", "dataset": "published", "pin": "{}", catalogDiscoveryFlag: true}
	if err := prepareCatalogDiscovery(ws, "knowledge.search", flags); err != nil {
		t.Fatal(err)
	}
	if !isCatalogDiscovery(flags) {
		t.Fatal("validated context not established")
	}
	for _, action := range []string{"knowledge.read", "knowledge.rerank", "file.read", "dataset.manage"} {
		if err := prepareCatalogDiscovery(ws, action, flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("discovery context bypassed %s: %v", action, err)
		}
	}
	flags["dataset"] = "private"
	if err := prepareCatalogDiscovery(ws, "knowledge.search", flags); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("unconfigured Workspace accepted %v", err)
	}
	flags["dataset"] = "published"
	delete(flags, "pin")
	if err := prepareCatalogDiscovery(ws, "knowledge.search", flags); err == nil || !strings.Contains(err.Error(), "pin") {
		t.Fatalf("search without fixed pin accepted %v", err)
	}
}

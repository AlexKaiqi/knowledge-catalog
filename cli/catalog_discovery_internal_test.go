package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKnowledgeSearchRejectsInventoryDiscoveryOperand(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("search --catalog must not reach HTTP")
	}))
	defer server.Close()
	result := Run([]string{"search", "--catalog", "retired", "--query", "note", "--server", server.URL, "--as", "viewer"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
		t.Fatalf("catalog discovery operand must be rejected: %s", result.Stdout)
	}
}

func TestProductCLIRejectsRetiredCatalogBeforeModuleValidation(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	for _, args := range [][]string{
		{"log", "--repo", "kr://acme/source", "--object", "note/one"},
		{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fone"},
		{"search", "--query", "note"},
		{"operations", "projection", "sync", "--repo", "kr://acme/source"},
		{"access", "--repo", "kr://acme/source", "--object", "note/one", "--aspect", "preview"},
		{"dataset", "define", "--dataset", "agent"},
	} {
		t.Run(strings.Join(args[:1], " "), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("retired --catalog must not reach HTTP")
			}))
			defer server.Close()
			argv := append([]string{"--server", server.URL, "--as", "viewer"}, args...)
			result := Run(append(argv, "--catalog", "retired"))
			if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") || !strings.Contains(result.Stdout, "rejects --catalog") {
				t.Fatalf("global catalog rejection lost to module validation: %s", result.Stdout)
			}
		})
	}
}

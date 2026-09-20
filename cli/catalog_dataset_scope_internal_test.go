package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kcclient "kc/client"
	"kc/kernel"
)

// A selected Catalog is client context. An explicit Dataset must still reach
// the Dataset search contract, including its ordinary authorization failures,
// without requiring or requesting the Catalog's discovery Workspace.
func TestDatasetSearchKeepsExplicitScopeWithSelectedCatalog(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("KC_CONFIG_DIR", t.TempDir())
			t.Setenv("KC_HOME", t.TempDir())
			t.Setenv("KC_AUTH_TOKEN", "")
			t.Setenv("KC_AS", "")
			t.Setenv("KC_CATALOG", "")
			t.Setenv("KC_DATASET", "")
			var paths []string
			var search kcclient.KnowledgeSearchRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodPost || r.URL.Path != "/knowledge/v1/search" {
					// This Catalog deliberately has no discoveryWorkspaceId.
					_ = json.NewEncoder(w).Encode(map[string]any{"catalogId": "kr://scene/catalog"})
					return
				}
				if r.Header.Get("X-Kc-As") != "reader" {
					t.Errorf("search lost the explicit principal: %q", r.Header.Get("X-Kc-As"))
				}
				if err := json.NewDecoder(r.Body).Decode(&search); err != nil {
					t.Error(err)
				}
				w.WriteHeader(status)
				if status == http.StatusForbidden {
					_ = json.NewEncoder(w).Encode(kernel.FaultJSON(kernel.Fail(kernel.ErrForbidden, "Dataset file.read required")))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"hits": []any{}})
			}))
			defer server.Close()
			if err := persistClientCatalog(server.URL, "kr://scene/catalog"); err != nil {
				t.Fatal(err)
			}
			result := Run([]string{"search", "--dataset", "scene-set", "--query", "merchandise", "--server", server.URL, "--as", "reader"})
			if status == http.StatusOK && result.Status != 0 {
				t.Fatalf("explicit Dataset search required Catalog discovery: %s; requests=%v", result.Stdout, paths)
			}
			if status == http.StatusForbidden && (result.Status != 1 || !strings.Contains(result.Stdout, "FORBIDDEN") || !strings.Contains(result.Stdout, "Dataset file.read required")) {
				t.Fatalf("Dataset authorization failure was replaced: %s; requests=%v", result.Stdout, paths)
			}
			if len(paths) != 1 || paths[0] != "POST /knowledge/v1/search" {
				t.Fatalf("explicit Dataset triggered additional discovery or resolution requests: %v", paths)
			}
			if search.Catalog != "kr://scene/catalog" || search.Dataset != "scene-set" || search.Query != "merchandise" || search.CatalogDiscovery || len(search.Pin) != 0 || search.Definition != nil {
				t.Fatalf("explicit Dataset search scope changed: %#v", search)
			}
		})
	}
}

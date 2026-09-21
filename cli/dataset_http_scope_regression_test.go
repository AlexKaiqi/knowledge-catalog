package cli_test

import (
	"encoding/json"
	"kc/cli"
	"kc/internal/testkit"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatasetRegressionHTTPDatasetPinCannotBroadenScope(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID, repo, principal := "kr://review/catalog", "kr://review/source", "agent:reviewer"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "repository", "attach", "--repo", repo))
	body(t, kc(home, "attach", "--repo", repo))
	body(t, kc(home, "writer", "put", "--command-id", "public", "--repo", repo, "--object", "public", "--path-hint", "public/a.yaml", "--value", `{"body":"public"}`))
	body(t, kc(home, "writer", "put", "--command-id", "private", "--repo", repo, "--object", "secret", "--path-hint", "private/a.yaml", "--value", `{"body":"OUTSIDE_DATASET"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "public-only", "--revision", "1", "--source", repo+"=refs/heads/main@docs@public"))
	body(t, kc(home, "grant", "add", "--principal", principal, "--action", "dataset.resolve,file.read", "--catalog", catalogID, "--dataset", "public-only"))
	handler := cli.HTTPHandler(home)
	if c, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = c.Close() })
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, resolved, _ := httpSurfaceRequest(t, server, http.MethodPost, "/dataset-files/v1/mounts:list", map[string]any{"catalog": catalogID, "dataset": "public-only"}, principal)
	if status != http.StatusOK {
		t.Fatalf("setup resolve %d %#v", status, resolved)
	}
	pin := asMap(t, asMap(t, resolved)["pin"])
	items := pin["items"].([]any)
	asMap(t, items[0])["prefix"] = ""
	status, result, _ := httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/objects:read", map[string]any{"catalog": catalogID, "dataset": "public-only", "pin": pin, "object": "secret"}, principal)
	if status == http.StatusOK {
		raw := string(mustReviewJSON(result))
		if strings.Contains(raw, "OUTSIDE_DATASET") {
			t.Fatalf("Dataset-only caller read outside scope: %s", raw)
		}
	}
}
func mustReviewJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }

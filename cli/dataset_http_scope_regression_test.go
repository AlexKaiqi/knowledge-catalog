package cli_test

import (
	"kc/internal/testkit"
	"kc/kernel"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatasetRegressionHTTPDatasetPinCannotBroadenScope(t *testing.T) {
	home := testkit.TempDir(t)
	repo, principal := "kr://review/source", "agent:reviewer"
	body(t, kc(home, "local", "init"))
	repoDSN := lakeFSRepoDSN(t)
	body(t, kc(home, "local", "repository", "attach", "--repo", repo, "--dsn", repoDSN))
	body(t, kc(home, "attach", "--repo", repo, "--dsn", repoDSN))
	body(t, kc(home, "writer", "put", testObjectReference(t, repo, "public", ""), "--command-id", "public", "--path-hint", "public/a.yaml", "--value", `{"body":"public"}`))
	body(t, kc(home, "writer", "put", testObjectReference(t, repo, "secret", ""), "--command-id", "private", "--path-hint", "private/a.yaml", "--value", `{"body":"OUTSIDE_DATASET"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "public-only", "--revision", "1", "--source", repo+"=refs/heads/main@docs@public"))
	body(t, kc(home, "grant", "add", "--principal", principal, "--action", "dataset.resolve,file.read", "--dataset", "public-only"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if c, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = c.Close() })
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, resolved, _ := httpSurfaceRequest(t, server, http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": "public-only"}, principal)
	if status != http.StatusOK {
		t.Fatalf("setup resolve %d %#v", status, resolved)
	}
	pin := asMap(t, asMap(t, resolved)["pin"])
	status, result, _ := httpSurfaceRequest(t, server, http.MethodPost, "/object/v1/objects:read", map[string]any{"dataset": "public-only", "pin": pin, "reference": testObjectReference(t, repo, "public", "")}, principal)
	if status != http.StatusOK {
		t.Fatalf("valid published object was not readable: HTTP %d %#v", status, result)
	}
	row := asMap(t, result)
	if row["reference"] != testObjectReference(t, repo, "public", "") || asMap(t, row["address"])["objectId"] != "public" || row["repository"] != repo ||
		row["commit"] != asMap(t, pin["repositories"])[repo] || asMap(t, row["value"])["body"] != "public" {
		t.Fatalf("valid pin lost its published object, body or source basis: %#v", result)
	}
	items := pin["items"].([]any)
	asMap(t, items[0])["prefix"] = ""
	status, result, _ = httpSurfaceRequest(t, server, http.MethodPost, "/object/v1/objects:read", map[string]any{"dataset": "public-only", "pin": pin, "reference": testObjectReference(t, repo, "secret", "")}, principal)
	if status != http.StatusBadRequest || asMap(t, asMap(t, result)["error"])["code"] != string(kernel.ErrDatasetInvalid) {
		t.Fatalf("forged Dataset pin did not fail its scope contract: HTTP %d %#v", status, result)
	}
}

package cli_test

import (
	"encoding/base64"
	"gopkg.in/yaml.v3"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/semanticview"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatasetRegressionSemanticVFSMustRespectDatasetScope(t *testing.T) {
	home := testkit.TempDir(t)
	repo, principal := "kr://review/vfs/source", "agent:reviewer"
	body(t, kc(home, "local", "init"))
	repoDSN := lakeFSRepoDSN(t)
	body(t, kc(home, "local", "repository", "attach", "--repo", repo, "--dsn", repoDSN))
	body(t, kc(home, "attach", "--repo", repo, "--dsn", repoDSN))
	body(t, kc(home, "writer", "put", testObjectReference(t, repo, "public", ""), "--command-id", "public", "--path-hint", "public/a.yaml", "--value", `{"body":"public"}`))
	body(t, kc(home, "writer", "put", testObjectReference(t, repo, "secret", ""), "--command-id", "private", "--path-hint", "private/a.yaml", "--value", `{"body":"OUTSIDE_VFS_DATASET"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "public-only", "--revision", "1", "--source", repo+"=refs/heads/main@docs@public"))
	body(t, kc(home, "grant", "add", "--principal", principal, "--action", "dataset.resolve,file.read", "--dataset", "public-only"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if c, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = c.Close() })
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, result, _ := httpSurfaceRequest(t, server, http.MethodPost, "/snapshot/v1/files/mounts:list", map[string]any{"dataset": "public-only", "view": "semantic"}, principal)
	if status != http.StatusOK {
		t.Fatalf("setup mount %d %#v", status, result)
	}
	mounts := asMap(t, result)
	mount := asMap(t, mounts["mounts"].([]any)[0])["path"]
	request := map[string]any{"dataset": "public-only", "view": "semantic", "pin": mounts["pin"], "mountPath": mount, "directory": "objects"}
	status, result, _ = httpSurfaceRequest(t, server, http.MethodPost, "/snapshot/v1/files/tree:list", request, principal)
	if status != http.StatusOK {
		t.Fatalf("list %d %#v", status, result)
	}
	entries := asMap(t, result)["entries"].([]any)
	if len(entries) != 1 || asMap(t, entries[0])["kind"] != "file" || !strings.HasPrefix(asMap(t, entries[0])["name"].(string), "public--") || asMap(t, result)["exhausted"] != true {
		t.Fatalf("semantic listing must expose exactly the permitted file: %#v", result)
	}
	delete(request, "directory")
	request["file"] = "objects/" + asMap(t, entries[0])["name"].(string)
	status, result, _ = httpSurfaceRequest(t, server, http.MethodPost, "/snapshot/v1/files/file:read", request, principal)
	if status != http.StatusOK {
		t.Fatalf("permitted semantic file was not readable: HTTP %d %#v", status, result)
	}
	raw, err := base64.StdEncoding.DecodeString(asMap(t, result)["content"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	meta := asMap(t, document["_kc"])
	if document["body"] != "public" || meta["object_id"] != "public" || meta["repository"] != repo ||
		meta["commit"] != asMap(t, asMap(t, mounts["pin"])["repositories"])[repo] {
		t.Fatalf("semantic file lost its permitted content or frozen source: %s", raw)
	}
	// The private object's deterministic semantic address is still forbidden
	// when requested directly, even though it was absent from enumeration.
	request["file"] = semanticview.Path(objectmodel.ObjectValue{Address: objectmodel.Address{ObjectID: "secret"}}, "")
	status, result, _ = httpSurfaceRequest(t, server, http.MethodPost, "/snapshot/v1/files/file:read", request, principal)
	if status != http.StatusNotFound || asMap(t, asMap(t, result)["error"])["code"] != string(kernel.ErrObjectRefUnresolved) {
		t.Fatalf("excluded semantic file did not fail its scope contract: HTTP %d %#v", status, result)
	}
}

package cli_test

import (
	"encoding/base64"
	"kc/cli"
	"kc/internal/testkit"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatasetRegressionSemanticVFSMustRespectDatasetScope(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID, repo, principal := "kr://review/vfs/catalog", "kr://review/vfs/source", "agent:reviewer"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	repoDSN := lakeFSRepoDSN(t)
	body(t, kc(home, "local", "repository", "attach", "--repo", repo, "--dsn", repoDSN))
	body(t, kc(home, "attach", "--repo", repo, "--dsn", repoDSN))
	body(t, kc(home, "writer", "put", "--command-id", "public", "--repo", repo, "--object", "public", "--path-hint", "public/a.yaml", "--value", `{"body":"public"}`))
	body(t, kc(home, "writer", "put", "--command-id", "private", "--repo", repo, "--object", "secret", "--path-hint", "private/a.yaml", "--value", `{"body":"OUTSIDE_VFS_DATASET"}`))
	body(t, kc(home, "dataset", "define", "--dataset", "public-only", "--revision", "1", "--source", repo+"=refs/heads/main@docs@public"))
	body(t, kc(home, "grant", "add", "--principal", principal, "--action", "dataset.resolve,file.read", "--catalog", catalogID, "--dataset", "public-only"))
	handler := cli.HTTPHandler(home)
	if c, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = c.Close() })
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, result, _ := httpSurfaceRequest(t, server, http.MethodPost, "/dataset-files/v1/mounts:list", map[string]any{"catalog": catalogID, "dataset": "public-only", "view": "semantic"}, principal)
	if status != http.StatusOK {
		t.Fatalf("setup mount %d %#v", status, result)
	}
	mounts := asMap(t, result)
	mount := asMap(t, mounts["mounts"].([]any)[0])["path"]
	request := map[string]any{"catalog": catalogID, "dataset": "public-only", "view": "semantic", "pin": mounts["pin"], "mountPath": mount, "directory": "objects"}
	status, result, _ = httpSurfaceRequest(t, server, http.MethodPost, "/dataset-files/v1/tree:list", request, principal)
	if status != http.StatusOK {
		t.Fatalf("list %d %#v", status, result)
	}
	foundPublic := false
	for _, entry := range asMap(t, result)["entries"].([]any) {
		name := asMap(t, entry)["name"].(string)
		if strings.HasPrefix(name, "public--") {
			foundPublic = true
		}
		if !strings.HasPrefix(name, "secret--") {
			continue
		}
		delete(request, "directory")
		request["file"] = "objects/" + name
		status, result, _ = httpSurfaceRequest(t, server, http.MethodPost, "/dataset-files/v1/file:read", request, principal)
		if status == http.StatusOK {
			raw, _ := base64.StdEncoding.DecodeString(asMap(t, result)["content"].(string))
			if strings.Contains(string(raw), "OUTSIDE_VFS_DATASET") {
				t.Fatalf("semantic VFS returned excluded file content: %s", raw)
			}
		}
	}
	if !foundPublic {
		t.Fatalf("semantic scope discarded the permitted object: %#v", result)
	}
}

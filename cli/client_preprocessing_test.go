package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/catalog"
	"kc/cli"
)

func TestWorkspaceOverlayProducesPortableRecipeOffline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("preprocessing reached KC Server") }))
	defer server.Close()
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("DSH_TASK_ID", "unbound-task")
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	t.Setenv("KC_HOME", filepath.Join(root, "must-not-exist"))
	base := filepath.Join(root, "base.yaml")
	overlay := filepath.Join(root, "overlay.yaml")
	out := filepath.Join(root, "merged.yaml")
	baseBytes := []byte("name: incident\nmounts:\n  - repository: kr://acme/team\n    selector: refs/heads/main\n    path: knowledge\n")
	if err := os.WriteFile(base, baseBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte("mounts:\n  - repository: kr://acme/policy\n    selector: refs/heads/release\n    path: policies\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(args ...string) kcRunResult { return kcRunResultFrom(cli.Run(args), publicCommandPath(args)) }
	definition := asMap(t, body(t, call("workspace", "overlay", "--file", base, "--overlay", overlay)))
	if definition["workspaceId"] != "incident" || len(definition["sources"].([]any)) != 2 {
		t.Fatalf("merged definition: %#v", definition)
	}
	receipt := asMap(t, body(t, call("workspace", "overlay", "--file", base, "--overlay", overlay, "--out", out)))
	if receipt["out"] != out || receipt["workspaceId"] != "incident" {
		t.Fatal(receipt)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := catalog.ParseWorkspaceRecipe(raw)
	if err != nil || recipe.Name != "incident" || len(recipe.Mounts) != 2 || recipe.Mounts[1].Selector != "refs/heads/release" {
		t.Fatalf("portable recipe: %#v %v", recipe, err)
	}
	if got, err := os.ReadFile(base); err != nil || !reflect.DeepEqual(got, baseBytes) {
		t.Fatalf("modified shared recipe: %s %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("preprocessing opened Home: %v", err)
	}
	t.Run("missing overlay", func(t *testing.T) { expectCode(t, call("workspace", "overlay", "--file", base), "USAGE_INVALID") })
	t.Run("invalid removal", func(t *testing.T) {
		if err := os.WriteFile(overlay, []byte("remove: [kr://acme/absent]\n"), 0600); err != nil {
			t.Fatal(err)
		}
		expectCode(t, call("workspace", "overlay", "--file", base, "--overlay", overlay), "USAGE_INVALID")
	})
}

func TestDeploymentOperationsRejectMissingConfigAndCallerOverrides(t *testing.T) {
	for _, args := range [][]string{{"deployment", "init"}, {"deployment", "status"}, {"deployment", "system", "publish"}} {
		t.Run(publicCommandPath(args), func(t *testing.T) {
			expectCode(t, deploymentCommand(t, args...), "USAGE_INVALID")
			expectCode(t, deploymentCommand(t, append(append([]string{}, args...), "--config", "missing.yaml", "--home", t.TempDir())...), "USAGE_INVALID")
		})
	}
}

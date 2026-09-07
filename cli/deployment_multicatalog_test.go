package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"kc/cli"
	apphome "kc/home"
)

func TestDeploymentAddsCatalogExplicitlyAndRecoversIsolation(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	first := cfg.Catalogs[0].ID
	second := "kr://recover/restricted"
	remote := filepath.Join(filepath.Dir(path), "restricted.git")
	if raw, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("git: %s %v", raw, err)
	}
	cfg.Catalogs = append(cfg.Catalogs, apphome.CatalogBinding{ID: second, Remote: remote})
	writeDeployment(t, path, cfg)
	if h, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{}); err == nil {
		_ = h.(interface{ Close() error }).Close()
		t.Fatal("new Catalog configuration was initialized by Server startup")
	}
	before, err := os.ReadFile(filepath.Join(cfg.StateDir, "allow.json"))
	if err != nil {
		t.Fatal(err)
	}
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	if after, err := os.ReadFile(filepath.Join(cfg.StateDir, "allow.json")); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("adding a Catalog rewrote existing grants: %v", err)
	}
	start := func() (*httptest.Server, http.Handler) {
		h, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return httptest.NewServer(h), h
	}
	stop := func(server *httptest.Server, h http.Handler) {
		server.Close()
		if err := h.(interface{ Close() error }).Close(); err != nil {
			t.Fatal(err)
		}
	}
	server, h := start()
	t.Cleanup(func() {
		if server != nil {
			stop(server, h)
		}
	})
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "agent:operator", args...) }
	expectCode(t, call("workspace", "define", "public-task", "--revision", "1", "--source", "kr://kc/system"), "USAGE_INVALID")
	body(t, call("workspace", "define", "public-task", "--catalog", first, "--revision", "1", "--source", "kr://kc/system"))
	body(t, call("workspace", "define", "restricted-task", "--catalog", second, "--revision", "1", "--source", "kr://kc/system"))
	body(t, call("admin", "grant", "add", "--principal", "agent:restricted", "--action", "catalog.read", "--catalog", second))
	states := map[string]any{}
	for id, workspace := range map[string]string{first: "public-task", second: "restricted-task"} {
		state := asMap(t, body(t, call("catalog", "show", "--catalog", id)))
		workspaces := state["workspaces"].([]any)
		if len(workspaces) != 1 || asMap(t, workspaces[0])["workspaceId"] != workspace {
			t.Fatalf("Catalog %s contains another Catalog's workspace: %#v", id, state)
		}
		states[id] = state
	}
	stop(server, h)
	server = nil
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	server, h = start()
	for _, id := range []string{first, second} {
		if got := body(t, call("catalog", "show", "--catalog", id)); !reflect.DeepEqual(states[id], got) {
			t.Fatalf("Catalog %s did not recover its own state", id)
		}
	}
	visible := asMap(t, body(t, kcRemote(t, server.URL, "agent:restricted", "catalog", "list")))["catalogs"].([]any)
	if len(visible) != 1 || asMap(t, visible[0])["id"] != second {
		t.Fatalf("Catalog discovery crossed grant scope: %#v", visible)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:restricted", "catalog", "show", "--catalog", first), "FORBIDDEN")
	body(t, kcRemote(t, server.URL, "agent:restricted", "catalog", "show", "--catalog", second))
}

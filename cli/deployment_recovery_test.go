package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"kc/cli"
	apphome "kc/home"
	"kc/hook"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func declaredDeployment(t *testing.T, source bool) (apphome.DeploymentConfig, string) {
	t.Helper()
	root := t.TempDir()
	authority := filepath.Join(root, "authority.git")
	if raw, err := exec.Command("git", "init", "--bare", authority).CombinedOutput(); err != nil {
		t.Fatalf("git: %s %v", raw, err)
	}
	cfg := apphome.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "instance"), Auth: "local", BootstrapPrincipal: "agent:operator", Catalogs: []apphome.CatalogBinding{{ID: "kr://recover/catalog", Remote: authority}}}
	if source {
		fixture := filepath.Join(root, "provisioning")
		if _, _, err := cli.InitHome(fixture, "kr://fixture/catalog"); err != nil {
			t.Fatal(err)
		}
		ws, err := cli.Open(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cli.AddRepository(ws, "kr://recover/source", "dolt", "", filepath.Join(root, "source"), ""); err != nil {
			t.Fatal(err)
		}
		if err := ws.Close(); err != nil {
			t.Fatal(err)
		}
		cfg.Repositories = []apphome.RepositoryBinding{{ID: "kr://recover/source", Driver: "dolt", Dir: filepath.Join(root, "source")}}
	}
	path := filepath.Join(root, "deployment.json")
	writeDeployment(t, path, cfg)
	return cfg, path
}
func writeDeployment(t *testing.T, path string, cfg apphome.DeploymentConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func deploymentCommand(t *testing.T, args ...string) kcRunResult {
	t.Helper()
	return kcRunResultFrom(cli.Run(args), publicCommandPath(args))
}

func TestDeploymentSurvivesInstanceReplacement(t *testing.T) {
	cfg, path := declaredDeployment(t, true)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	body(t, deploymentCommand(t, "deployment", "status", "--config", path))
	var server *httptest.Server
	var handler http.Handler
	start := func() {
		var err error
		handler, err = cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		server = httptest.NewServer(handler)
	}
	stop := func() {
		server.Close()
		if closer, ok := handler.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	start()
	defer func() {
		if server != nil {
			stop()
		}
	}()
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "agent:operator", args...) }
	initial := asMap(t, body(t, call("show")))
	if len(initial["repositories"].([]any)) != 1 {
		t.Fatalf("configuration admitted source implicitly: %#v", initial)
	}
	headBefore := body(t, call("writer", "head", "--repo", "kr://recover/source"))
	body(t, call("attach", "--repo", "kr://recover/source"))
	if got := body(t, call("writer", "head", "--repo", "kr://recover/source")); !reflect.DeepEqual(headBefore, got) {
		t.Fatalf("attach changed source: %#v => %#v", headBefore, got)
	}
	attached := body(t, call("show"))
	body(t, call("attach", "--repo", "kr://recover/source"))
	if got := body(t, call("show")); !reflect.DeepEqual(attached, got) {
		t.Fatal("repeat attach changed membership")
	}
	expectCode(t, call("attach", "--repo", "kr://recover/unconfigured"), "PRECONDITION_FAILED")
	body(t, call("workspace", "define", "incident", "--revision", "1", "--source", "kr://recover/source"))
	body(t, call("grant", "add", "--principal", "agent:reader", "--action", "knowledge.read", "--repo", "kr://recover/source"))
	body(t, call("operations", "gate", "add", "--on", "merge", "--repo", "kr://recover/source", "--require", "suite:recovery-suite"))
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer sink.Close()
	body(t, call("operations", "hook", "add", "--on", "writer.commit", "--phase", "post", "--url", sink.URL))
	receipt := body(t, call("writer", "put", "--command-id", "recovery-write", "--repo", "kr://recover/source", "--object", "note/recover", "--value", `{"text":"durable"}`))
	body(t, call("governance", "proposal", "create", "--proposal-id", "recovery-proposal", "--repo", "kr://recover/source", "--target", snapshot.DefaultRef, "--candidate", "refs/heads/candidates/recovery", "--object", "note/recover", "--value", `{"text":"proposed"}`))
	pin, err := json.Marshal(body(t, call("workspace", "pin", "--workspace", "incident")))
	if err != nil {
		t.Fatal(err)
	}
	preview := asMap(t, body(t, call("governance", "preview", "create", "--proposal", "recovery-proposal", "--pin", string(pin))))
	previewID := preview["previewId"].(string)
	body(t, call("governance", "preview", "validate", "--preview", previewID))
	validation := asMap(t, body(t, call("governance", "validation", "record", "--preview", previewID, "--suite", "recovery-suite", "--outcome", "PASSED")))
	oldHead := body(t, call("writer", "head", "--repo", "kr://recover/source"))
	catalogState := body(t, call("show"))
	policy := body(t, call("grant", "list"))
	gates := body(t, call("operations", "gate", "list"))
	stats, err := hook.InspectOutbox(cfg.StateDir)
	if err != nil || stats.Pending == 0 {
		t.Fatalf("missing durable outbox: %#v %v", stats, err)
	}
	stop()
	server = nil
	preserved := map[string][]byte{}
	for _, name := range []string{"allow.json", "gates.json", "hooks.json", "writer.db", "control.json", "system.jsonl", "access.jsonl", "hook-outbox.jsonl"} {
		raw, err := os.ReadFile(filepath.Join(cfg.StateDir, name))
		if err != nil {
			t.Fatal(err)
		}
		preserved[name] = raw
	}
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	// A new instance uses a new empty cache; only config and durable stores survive.
	cfg.CacheDir = filepath.Join(filepath.Dir(cfg.CacheDir), "replacement-instance")
	writeDeployment(t, path, cfg)
	start()
	for name, want := range preserved {
		got, err := os.ReadFile(filepath.Join(cfg.StateDir, name))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("startup rewrote %s: %v", name, err)
		}
	}
	if got := body(t, call("writer", "head", "--repo", "kr://recover/source")); !reflect.DeepEqual(got, oldHead) {
		t.Fatal("recovery changed knowledge HEAD")
	}
	if got := body(t, call("show")); !reflect.DeepEqual(got, catalogState) {
		t.Fatal("Catalog membership/recipe lost on replacement")
	}
	if got := body(t, call("grant", "list")); !reflect.DeepEqual(got, policy) {
		t.Fatal("grants lost on replacement")
	}
	if got := body(t, call("operations", "gate", "list")); !reflect.DeepEqual(got, gates) {
		t.Fatal("gates lost on replacement")
	}
	replay := asMap(t, body(t, call("writer", "put", "--command-id", "recovery-write", "--repo", "kr://recover/source", "--object", "note/recover", "--value", `{"text":"durable"}`)))
	if replay["disposition"] != "REPLAYED" {
		t.Fatalf("idempotency receipt lost: original=%#v replay=%#v", receipt, replay)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:reader", "writer", "put", "--command-id", "forbidden-recovery", "--repo", "kr://recover/source", "--object", "note/recover", "--value", `{}`), "FORBIDDEN")
	body(t, call("governance", "proposal", "merge", "--proposal", "recovery-proposal", "--preview", previewID, "--validation", validation["reportId"].(string)))
}

func TestDeploymentMissingDurableStateFailsClosed(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	defer handler.(interface{ Close() error }).Close()
	if err := os.Remove(filepath.Join(cfg.StateDir, "gates.json")); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:operator", "show"), "PRECONDITION_FAILED")
	if _, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("restart recreated missing policy: %v", err)
	}
	expectCode(t, deploymentCommand(t, "deployment", "init", "--config", path), "PRECONDITION_FAILED")
}

func TestDeploymentSystemPublishUsesDeclaredBinding(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	cfg.Repositories = []apphome.RepositoryBinding{{ID: string(knowledge.SystemRepositoryID), Driver: "dolt", Dir: filepath.Join(filepath.Dir(cfg.StateDir), "system")}}
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	first := asMap(t, body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", path)))
	if first["seeded"] != true {
		t.Fatalf("system publication not explicit: %#v", first)
	}
	second := asMap(t, body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", path)))
	if second["seeded"] != false || second["commit"] != first["commit"] {
		t.Fatalf("system publication not idempotent: %#v", second)
	}
	body(t, deploymentCommand(t, "deployment", "status", "--config", path))
}

func TestDeploymentReadinessRequiresCatalogAuthority(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	if err := os.Rename(cfg.Catalogs[0].Remote, cfg.Catalogs[0].Remote+".unavailable"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/readyz/consumer", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness ignored Catalog authority outage: %d %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["reasonCode"] != "CATALOG_STATE_UNAVAILABLE" {
		t.Fatalf("wrong dependency: %#v", result)
	}
}

package cli_test

import (
	"encoding/json"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/cli"
	apphome "kc/home"
	"kc/hook"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
)

func declaredDeployment(t *testing.T, source bool) (apphome.DeploymentConfig, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	fake := testkit.NewLakeFSFake(t)
	cfg := apphome.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "instance"), Auth: "local", BootstrapPrincipal: "agent:operator", Repositories: []apphome.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}, Registry: apphome.RegistryBinding{ID: "kr://kc/registry", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}, RepositoryAccess: []apphome.RepositoryAccess{apphome.SystemRepositoryAccess()}}
	if source {
		sourceDSN := fake.DSN(fake.NewRepo())
		cfg.Repositories = append(cfg.Repositories, apphome.RepositoryBinding{ID: "kr://recover/source", Driver: "lakefs", DSN: sourceDSN})
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
		handler, err = serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
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
	body(t, call("dataset", "define", "incident", "--revision", "1", "--source", "kr://recover/source"))
	body(t, call("grant", "add", "--principal", "agent:reader", "--action", "object.read", "--repo", "kr://recover/source"))
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer sink.Close()
	body(t, call("operations", "hook", "add", "--on", "writer.commit", "--phase", "post", "--url", sink.URL))
	body(t, call("operations", "hook", "add", "--on", "writer.merge", "--phase", "post", "--repo", "kr://recover/source", "--url", sink.URL))
	receipt := body(t, call("writer", "put", testObjectReference(t, "kr://recover/source", "note/recover", ""), "--repo", "kr://recover/source", "--command-id", "recovery-write", "--value", `{"text":"durable"}`))
	body(t, call("dataset", "define", "incident", "--revision", "2", "--source", "kr://recover/source"))
	body(t, call("writer", "branch", "--repo", "kr://recover/source", "--name", "review/recovery"))
	proposed := asMap(t, body(t, call("writer", "put", testObjectReference(t, "kr://recover/source", "note/recover", ""), "--repo", "kr://recover/source", "--command-id", "recovery-branch-write", "--ref", "refs/heads/review/recovery", "--value", `{"text":"proposed"}`)))
	reviewed := asMap(t, proposed["result"])["newCommit"].(string)
	oldHead := body(t, call("writer", "head", "--repo", "kr://recover/source"))
	catalogState := body(t, call("show"))
	policy := body(t, call("grant", "list"))
	hooks := body(t, call("operations", "hook", "list"))
	stats, err := hook.InspectOutbox(cfg.StateDir)
	if err != nil || stats.Pending == 0 {
		t.Fatalf("missing durable outbox: %#v %v", stats, err)
	}
	stop()
	server = nil
	preserved := map[string][]byte{}
	for _, name := range []string{"allow.json", "hooks.json", "writer.db", "system.jsonl", "access.jsonl", "hook-outbox.jsonl"} {
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
		t.Fatal("recovery changed object HEAD")
	}
	if got := body(t, call("show")); !reflect.DeepEqual(got, catalogState) {
		t.Fatal("Catalog membership/recipe lost on replacement")
	}
	if got := body(t, call("grant", "list")); !reflect.DeepEqual(got, policy) {
		t.Fatal("grants lost on replacement")
	}
	if got := body(t, call("operations", "hook", "list")); !reflect.DeepEqual(got, hooks) {
		t.Fatal("hooks lost on replacement")
	}
	replay := asMap(t, body(t, call("writer", "put", testObjectReference(t, "kr://recover/source", "note/recover", ""), "--repo", "kr://recover/source", "--command-id", "recovery-write", "--value", `{"text":"durable"}`)))
	if replay["disposition"] != "REPLAYED" {
		t.Fatalf("idempotency receipt lost: original=%#v replay=%#v", receipt, replay)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:reader", "writer", "put", testObjectReference(t, "kr://recover/source", "note/recover", ""), "--repo", "kr://recover/source", "--command-id", "forbidden-recovery", "--value", `{}`), "FORBIDDEN")
	merged := asMap(t, body(t, call("writer", "merge", "--command-id", "recovery-merge", "--repo", "kr://recover/source", "--from", "review/recovery", "--commit", reviewed)))
	if asMap(t, merged["result"])["commitId"] != reviewed {
		t.Fatalf("the reviewed branch commit did not survive replacement: %#v", merged)
	}
}

func TestDeploymentMissingDurableStateFailsClosed(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	defer handler.(interface{ Close() error }).Close()
	if err := os.Remove(filepath.Join(cfg.StateDir, "hooks.json")); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:operator", "show"), "PRECONDITION_FAILED")
	if _, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("restart recreated missing policy: %v", err)
	}
	expectCode(t, deploymentCommand(t, "deployment", "init", "--config", path), "PRECONDITION_FAILED")
}

func TestDeploymentSystemPublishUsesDeclaredBinding(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	fake := testkit.NewLakeFSFake(t)
	cfg.Repositories = []apphome.RepositoryBinding{{ID: string(objectmodel.SystemRepositoryID), Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	first := asMap(t, body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", path)))
	if first["seeded"] != false {
		t.Fatalf("deployment init did not publish system: %#v", first)
	}
	second := asMap(t, body(t, deploymentCommand(t, "deployment", "system", "publish", "--config", path)))
	if second["seeded"] != false || second["commit"] != first["commit"] {
		t.Fatalf("system publication not idempotent: %#v", second)
	}
	body(t, deploymentCommand(t, "deployment", "status", "--config", path))
}

func TestDeploymentReadinessRequiresCatalogAuthority(t *testing.T) {
	root := t.TempDir()
	fake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	cfg := apphome.DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "durable"), CacheDir: filepath.Join(root, "instance"), Auth: "local", BootstrapPrincipal: "agent:operator", Repositories: []apphome.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}}, Registry: apphome.RegistryBinding{ID: "kr://kc/registry", Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}, RepositoryAccess: []apphome.RepositoryAccess{apphome.SystemRepositoryAccess()}}
	path := filepath.Join(root, "deployment.json")
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	handler, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	// The authority endpoint going dark is the lakeFS form of a lost Catalog
	// authority; readiness must fail closed instead of serving from cache.
	fake.Close()
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
	if result["reasonCode"] != "REGISTRY_STATE_UNAVAILABLE" {
		t.Fatalf("wrong dependency: %#v", result)
	}
}

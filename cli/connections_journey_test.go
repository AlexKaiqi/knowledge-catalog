package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"kc/cli"
	apphome "kc/home"
)

// Public CLI -> typed Client -> actual Server -> durable binding and Catalog.
// The provider boundary rejects every mutation, and exposes controlled expiry.
func TestRepositoryConnectionCLIRecoversExpiredCredentialsWithoutRebinding(t *testing.T) {
	var mu sync.Mutex
	token, backend := "external-original-secret", 41
	requests := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.Method != "GET" {
			t.Errorf("external mutation %s", r.Method)
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("Authorization") != "token "+token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(token))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/repos/kaiqidong/existing":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": backend, "default_branch": "main", "empty": false})
		case "/api/v1/repos/kaiqidong/existing/branches/main":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": "initial"}})
		case "/api/v1/repos/kaiqidong/existing/git/commits/initial":
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "initial"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	cfg, config := declaredDeployment(t, false)
	cfg.Connections = &apphome.ConnectionPolicy{AllowedOrigins: []string{provider.URL}, CreatorActions: []string{"repository.connections.manage", "repository.metadata.read"}}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	var server *httptest.Server
	var handler http.Handler
	start := func() {
		var err error
		handler, err = cli.HTTPHandlerFromConfig(config, cli.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		server = httptest.NewServer(handler)
	}
	stop := func() {
		server.Close()
		server = nil
		if err := handler.(interface{ Close() error }).Close(); err != nil {
			t.Fatal(err)
		}
	}
	start()
	t.Cleanup(func() {
		if server != nil {
			stop()
		}
	})
	govern := func(args ...string) kcRunResult { return kcRemote(t, server.URL, cfg.BootstrapPrincipal, args...) }
	provide := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "kaiqidong", args...) }
	other := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "other", args...) }
	cat, repo := cfg.Catalogs[0].ID, "kr://kaiqidong/existing"
	body(t, govern("admin", "grant", "add", "--principal", "kaiqidong", "--catalog", cat, "--action", "catalog.repositories.connect"))
	secretFile := filepath.Join(t.TempDir(), "credential")
	putCredential := func(value string) {
		t.Helper()
		if err := os.WriteFile(secretFile, []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	putCredential(token)
	args := []string{"catalog", "repo", "connect", "--catalog", cat, "--repo", repo, "--url", provider.URL + "/kaiqidong/existing", "--credential-file", secretFile}
	expectCode(t, other(args...), "FORBIDDEN")
	deniedOrigin := append([]string(nil), args...)
	for i, v := range deniedOrigin {
		if v == "--url" {
			deniedOrigin[i+1] = "http://unapproved.invalid/alice/private"
		}
	}
	expectCode(t, provide(deniedOrigin...), "FORBIDDEN")
	mu.Lock()
	if requests != 0 {
		t.Error("unapproved provider was contacted")
	}
	mu.Unlock()
	created := asMap(t, body(t, kcRemote(t, server.URL, "kaiqidong", "catalog", "repo", "connect", "--catalog", cat, "--repo", repo, "--url", provider.URL+"/kaiqidong/existing", "--credential-file", secretFile)))
	if created["status"] != "READY" || created["managementURL"] != provider.URL+"/kaiqidong/existing" || created["head"] != "initial" {
		t.Fatalf("connection not reviewable %#v", created)
	}
	body(t, kcRemote(t, server.URL, "kaiqidong", "catalog", "repo", "connection", "show", "--repo", repo))
	body(t, kcRemote(t, server.URL, "kaiqidong", "catalog", "repo", "connection", "check", "--repo", repo))
	expectCode(t, other("catalog", "repo", "connection", "show", "--repo", repo), "FORBIDDEN")
	expectCode(t, other("catalog", "repo", "connection", "check", "--repo", repo), "FORBIDDEN")
	expectCode(t, other("catalog", "repo", "connection", "rotate", "--repo", repo, "--credential-file", secretFile), "FORBIDDEN")
	putCredential("wrong-secret")
	bad := provide("catalog", "repo", "connection", "rotate", "--repo", repo, "--credential-file", secretFile)
	expectCode(t, bad, "PRECONDITION_FAILED")
	if strings.Contains(bad.Stdout, token) || strings.Contains(bad.Stdout, "wrong-secret") {
		t.Fatal("provider or caller secret leaked in fault")
	}
	body(t, provide("catalog", "repo", "connection", "check", "--repo", repo))
	expectCode(t, provide("knowledge", "read", "--repo", repo, "--commit", "initial", "--object", "any"), "FORBIDDEN")
	stop()
	mu.Lock()
	token = "external-rotated-secret"
	before := requests
	mu.Unlock()
	cfg.CacheDir = filepath.Join(t.TempDir(), "new-cache")
	writeDeployment(t, config, cfg)
	start()
	mu.Lock()
	if requests != before {
		t.Error("Server recovery required source availability")
	}
	mu.Unlock()
	body(t, provide("catalog", "repo", "connection", "show", "--repo", repo))
	expectCode(t, provide("catalog", "repo", "connection", "check", "--repo", repo), "PRECONDITION_FAILED")
	putCredential(token)
	rotated := asMap(t, body(t, kcRemote(t, server.URL, "kaiqidong", "catalog", "repo", "connection", "rotate", "--repo", repo, "--credential-file", secretFile)))
	if rotated["managementURL"] != created["managementURL"] || rotated["head"] != created["head"] {
		t.Fatal("rotation changed authority")
	}
	body(t, provide("catalog", "repo", "connection", "check", "--repo", repo))
	// Revoking initial rights survives a successful connect replay.
	policy, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range policy.Rules {
		if rule.Principal == "kaiqidong" && rule.Repo == repo {
			body(t, govern("admin", "grant", "remove", "--id", rule.ID))
		}
	}
	body(t, provide(args...))
	expectCode(t, provide("catalog", "repo", "connection", "show", "--repo", repo), "FORBIDDEN")
	expectCode(t, provide("catalog", "repo", "connection", "rotate", "--repo", repo, "--credential-file", secretFile), "FORBIDDEN")
	for _, name := range []string{"allow.json", "system.jsonl", "audit.jsonl", "access.jsonl"} {
		raw, err := os.ReadFile(filepath.Join(cfg.StateDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"external-original-secret", "external-rotated-secret", "wrong-secret"} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("credential leaked into %s", name)
			}
		}
	}
}

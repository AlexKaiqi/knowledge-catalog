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
	cat, repo := cfg.Catalogs[0].ID, "kr://kaiqidong/existing"
	body(t, govern("grant", "add", "--principal", "kaiqidong", "--catalog", cat, "--action", "catalog.repositories.connect"))
	secretFile := filepath.Join(t.TempDir(), "credential")
	putCredential := func(value string) {
		t.Helper()
		if err := os.WriteFile(secretFile, []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	putCredential(token)
	connectURL := provider.URL + "/kaiqidong/existing"
	repositoryConnectExpect(t, server.URL, "other", cat, repo, connectURL, secretFile, "FORBIDDEN")
	repositoryConnectExpect(t, server.URL, "kaiqidong", cat, repo, "http://unapproved.invalid/alice/private", secretFile, "FORBIDDEN")
	mu.Lock()
	if requests != 0 {
		t.Error("unapproved provider was contacted")
	}
	mu.Unlock()
	created := repositoryConnect(t, server.URL, "kaiqidong", cat, repo, connectURL, secretFile)
	if created["status"] != "READY" || created["managementURL"] != connectURL || created["head"] != "initial" {
		t.Fatalf("connection not reviewable %#v", created)
	}
	repositoryConnectionShow(t, server.URL, "kaiqidong", repo)
	repositoryConnectionCheck(t, server.URL, "kaiqidong", repo)
	repositoryConnectionShowExpect(t, server.URL, "other", repo, "FORBIDDEN")
	repositoryConnectionCheckExpect(t, server.URL, "other", repo, "FORBIDDEN")
	repositoryConnectionRotateExpect(t, server.URL, "other", repo, secretFile, "FORBIDDEN")
	putCredential("wrong-secret")
	repositoryConnectionRotateExpect(t, server.URL, "kaiqidong", repo, secretFile, "PRECONDITION_FAILED")
	repositoryConnectionCheck(t, server.URL, "kaiqidong", repo)
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
	repositoryConnectionShow(t, server.URL, "kaiqidong", repo)
	repositoryConnectionCheckExpect(t, server.URL, "kaiqidong", repo, "PRECONDITION_FAILED")
	putCredential(token)
	rotated := repositoryConnectionRotate(t, server.URL, "kaiqidong", repo, secretFile)
	if rotated["managementURL"] != created["managementURL"] || rotated["head"] != created["head"] {
		t.Fatal("rotation changed authority")
	}
	repositoryConnectionCheck(t, server.URL, "kaiqidong", repo)
	// Revoking initial rights survives a successful connect replay.
	policy, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range policy.Rules {
		if rule.Principal == "kaiqidong" && rule.Repo == repo {
			body(t, govern("grant", "remove", "--id", rule.ID))
		}
	}
	repositoryConnect(t, server.URL, "kaiqidong", cat, repo, connectURL, secretFile)
	repositoryConnectionShowExpect(t, server.URL, "kaiqidong", repo, "FORBIDDEN")
	repositoryConnectionRotateExpect(t, server.URL, "kaiqidong", repo, secretFile, "FORBIDDEN")
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

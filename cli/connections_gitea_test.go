package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/cli"
	apphome "kc/home"
	"kc/internal/testkit"
)

func TestRepositoryConnectionOnLiveGitea(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	// The user's repository exists before KC receives the connection request.
	// Fixture creation is deliberately separate from the connect operation.
	name := "connected-" + run
	raw, _ := json.Marshal(map[string]any{"name": name, "private": true, "auto_init": true, "default_branch": "main"})
	req, _ := http.NewRequest("POST", base+"/api/v1/user/repos", bytes.NewReader(raw))
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("fixture repository creation status %d", resp.StatusCode)
	}
	cfg, config := declaredDeployment(t, false)
	cfg.Connections = &apphome.ConnectionPolicy{AllowedOrigins: []string{base}, CreatorActions: []string{"repository.connections.manage", "writer.preview", "writer.commit", "knowledge.read"}}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	start := func() (*httptest.Server, http.Handler) {
		h, err := cli.HTTPHandlerFromConfig(config, cli.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return httptest.NewServer(h), h
	}
	server, handler := start()
	closeServer := func() {
		server.Close()
		if err := handler.(interface{ Close() error }).Close(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { closeServer() }()
	body(t, kcRemote(t, server.URL, cfg.BootstrapPrincipal, "grant", "add", "--principal", "kaiqidong", "--catalog", cfg.Catalogs[0].ID, "--action", "catalog.repositories.connect"))
	credential := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(credential, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	repo := "kr://live/connected"
	catalogID := cfg.Catalogs[0].ID
	invoke := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "kaiqidong", args...) }
	connected := repositoryConnect(t, server.URL, "kaiqidong", catalogID, repo, base+"/kc/"+name, credential)
	if connected["managementURL"] != base+"/kc/"+name || connected["head"] == "" {
		t.Fatalf("connection %#v", connected)
	}
	receipt := asMap(t, body(t, invoke("writer", "put", "--repo", repo, "--command-id", "connected-publish", "--object", "note/connection", "--value", `{"body":"connected through the actual Gitea authority"}`)))
	commit := publishedCommit(t, receipt)
	repositoryConnectionRotate(t, server.URL, "kaiqidong", repo, credential)
	closeServer()
	cfg.CacheDir = filepath.Join(t.TempDir(), "replacement-cache")
	writeDeployment(t, config, cfg)
	server, handler = start()
	repositoryConnectionCheck(t, server.URL, "kaiqidong", repo)
	row := asMap(t, body(t, invoke("knowledge", "read", "--repo", repo, "--commit", commit, "--object", "note/connection")))
	if row["commit"] != commit || asMap(t, row["value"])["body"] != "connected through the actual Gitea authority" {
		t.Fatalf("fixed publication lost %#v", row)
	}
}

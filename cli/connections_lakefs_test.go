package cli_test

import (
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
)

func TestRepositoryConnectionOnLakeFS(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	base, token, name := fake.Origin(), fake.Credential(), fake.NewRepo()
	cfg, config := declaredDeployment(t, false)
	cfg.Connections = &apphome.ConnectionPolicy{AllowedOrigins: []string{base}, CreatorActions: []string{"repository.connections.manage", "writer.preview", "writer.commit", "knowledge.read"}}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	start := func() (*httptest.Server, http.Handler) {
		h, err := serv.HTTPHandlerFromConfig(config, serv.HTTPServerOptions{})
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
	body(t, kcRemote(t, server.URL, cfg.BootstrapPrincipal, "grant", "add", "--principal", "kaiqidong", "--action", "repositories.connect"))
	credential := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(credential, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	repo := "kr://live/connected"
	catalogID := cfg.Registry.ID
	invoke := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "kaiqidong", args...) }
	connected := repositoryConnect(t, server.URL, "kaiqidong", catalogID, repo, base+"/"+name, credential)
	if connected["managementURL"] != base+"/"+name || connected["head"] == "" {
		t.Fatalf("connection %#v", connected)
	}
	receipt := asMap(t, body(t, invoke("writer", "put", testObjectReference(t, repo, "note/connection", ""), "--command-id", "connected-publish", "--value", `{"body":"connected through the actual lakeFS authority"}`)))
	commit := publishedCommit(t, receipt)
	repositoryConnectionRotate(t, server.URL, "kaiqidong", repo, credential)
	closeServer()
	cfg.CacheDir = filepath.Join(t.TempDir(), "replacement-cache")
	writeDeployment(t, config, cfg)
	server, handler = start()
	repositoryConnectionCheck(t, server.URL, "kaiqidong", repo)
	row := asMap(t, body(t, invoke("read", testObjectReference(t, repo, "note/connection", ""), "--commit", commit)))
	if row["commit"] != commit || asMap(t, row["value"])["body"] != "connected through the actual lakeFS authority" {
		t.Fatalf("fixed publication lost %#v", row)
	}
}

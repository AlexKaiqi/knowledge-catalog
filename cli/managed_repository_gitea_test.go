package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"kc/cli"
	apphome "kc/home"
	"kc/internal/testkit"
)

// This is the same public provider path against the remote managed authority.
// The deployment has a pool, but never contains the allocated repository.
func TestManagedRepositoryProviderOnLiveGitea(t *testing.T) {
	base, token, _ := testkit.GiteaEndpoint(t)
	t.Setenv("KC_GITEA_TOKEN", token)
	cfg, config := declaredDeployment(t, false)
	cfg.ManagedRepositories = &apphome.ManagedRepositoryConfig{Driver: "gitea", DSN: base + "/kc", CreatorActions: []string{"writer.preview", "writer.commit", "knowledge.read"}}
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
		if closer, ok := handler.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	start()
	t.Cleanup(func() {
		if server != nil {
			stop()
		}
	})
	body(t, kcRemote(t, server.URL, "agent:operator", "grant", "add", "--catalog", cfg.Catalogs[0].ID, "--principal", "user:provider", "--action", "catalog.repositories.create"))
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "user:provider", args...) }
	body(t, call("catalog", "use", cfg.Catalogs[0].ID))
	createArgs := []string{"create", "--name", "hosted"}
	created := asMap(t, body(t, call(createArgs...)))
	repo, _ := created["repositoryId"].(string)
	if created["status"] != "APPLIED" || repo == "" || created["head"] == "" {
		t.Fatalf("invalid managed creation: %#v", created)
	}
	putArgs := []string{"writer", "put", "--repo", repo, "--object", "note/live", "--command-id", "live-publish", "--if-absent", "--value", `{"text":"hosted on Gitea"}`}
	receipt := asMap(t, body(t, call(putArgs...)))
	commit := publishedCommit(t, receipt)
	stop()
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	start()
	replayed := asMap(t, body(t, call(createArgs...)))
	if replayed["status"] != "REPLAYED" || replayed["head"] != created["head"] {
		t.Fatalf("creation history was not restored: %#v", replayed)
	}
	writeReplay := asMap(t, body(t, call(putArgs...)))
	if writeReplay["disposition"] != "REPLAYED" || publishedCommit(t, writeReplay) != commit {
		t.Fatalf("Writer history was not restored: %#v", writeReplay)
	}
	row := asMap(t, body(t, call("knowledge", "read", "--repo", repo, "--object", "note/live", "--commit", commit)))
	if row["commit"] != commit || asMap(t, row["value"])["text"] != "hosted on Gitea" {
		t.Fatalf("hosted knowledge was lost: %#v", row)
	}
}

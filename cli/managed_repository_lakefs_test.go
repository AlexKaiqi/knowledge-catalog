package cli_test

import (
	apphome "kc/home"
	"kc/internal/testkit"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// This is the same public provider path against the remote managed authority.
// The deployment has a pool, but never contains the allocated repository.
func TestManagedRepositoryProviderOnLakeFS(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", fake.Credential())
	cfg, config := declaredDeployment(t, false)
	cfg.ManagedRepositories = &apphome.ManagedRepositoryConfig{Driver: "lakefs", DSN: fake.Origin(), Root: "s3://kc-authority", PublicURL: "https://kc.example.test", CreatorActions: []string{"writer.preview", "writer.commit", "knowledge.read"}}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	var server *httptest.Server
	var handler http.Handler
	start := func() {
		var err error
		handler, err = serv.HTTPHandlerFromConfig(config, serv.HTTPServerOptions{})
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
	body(t, kcRemote(t, server.URL, "agent:operator", "grant", "add", "--principal", "provider", "--action", "repositories.create"))
	body(t, kcRemote(t, server.URL, "agent:operator", "grant", "add", "--principal", "provider", "--action", "inventory.read"))
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, "provider", args...) }
	createArgs := []string{"create", "--name", "hosted"}
	created := asMap(t, body(t, call(createArgs...)))
	repo, _ := created["repositoryId"].(string)
	if created["status"] != "APPLIED" || repo == "" || created["head"] == "" {
		t.Fatalf("invalid managed creation: %#v", created)
	}
	putArgs := []string{"writer", "put", "--repo", repo, "--object", "note/live", "--command-id", "live-publish", "--if-absent", "--value", `{"text":"hosted on lakeFS"}`}
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
	row := asMap(t, body(t, call("read", "--repo", repo, "--object", "note/live", "--commit", commit)))
	if row["commit"] != commit || asMap(t, row["value"])["text"] != "hosted on lakeFS" {
		t.Fatalf("hosted knowledge was lost: %#v", row)
	}
}

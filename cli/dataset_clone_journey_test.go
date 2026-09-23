package cli_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	apphome "kc/home"
	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// TestDatasetCloneJourney is the named formal argv journey for
// `kc dataset clone`: define with a directory mapping through the public CLI,
// then materialize the delivered tree as an ordinary local directory. The
// deep gateway assertions live in the internal delivery tests; this journey
// proves the argv surface reaches them.
func TestDatasetCloneJourney(t *testing.T) {
	lakefs := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", lakefs.Credential())
	repo := "kr://journey/docs"
	root := t.TempDir()
	cfg := apphome.DeploymentConfig{
		Version: 1, StateDir: filepath.Join(root, "state"), CacheDir: filepath.Join(root, "cache"),
		Auth: "local", BootstrapPrincipal: "agent:operator", Stores: apphome.StoresFile{Index: "none"},
		Catalogs:     []apphome.CatalogBinding{{ID: "kr://journey/catalog", Driver: "lakefs", DSN: lakefs.DSN(lakefs.NewRepo())}},
		Repositories: []apphome.RepositoryBinding{{ID: repo, Driver: "lakefs", DSN: lakefs.DSN(lakefs.NewRepo())}},
	}
	if err := apphome.InitializeDeployment(cfg, func(dir, principal string) error {
		return cli.WriteAllow(dir, cli.AllowFile{Rules: []cli.AllowRule{
			{ID: "operator", Principal: principal, Actions: []string{"*"}},
		}})
	}); err != nil {
		t.Fatal(err)
	}
	// Seed one source file through the deployment's TreeWriter.
	opened, err := apphome.OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	source, err := opened.Store.Require(kernel.RepositoryID(repo), kernel.ErrUsageInvalid)
	if err != nil {
		t.Fatal(err)
	}
	base, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.TreeWriter.Commit("seed-docs", snapshot.TreeChangeSet{
		TargetRepository: kernel.RepositoryID(repo), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Changes: []snapshot.TreeChange{{Path: "handbook/policies/README.md", Content: []byte("policy v1\n")}},
	}); err != nil {
		t.Fatal(err)
	}
	opened.Close()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "deployment.json")
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := cli.HTTPHandlerFromConfig(configPath, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handler.(interface{ Close() error }).Close() }()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Attach and publish through the public argv surface.
	body(t, kcRemote(t, server.URL, "agent:operator", "attach", "--repo", repo))
	body(t, kcRemote(t, server.URL, "agent:operator", "dataset", "define", "delivery",
		"--revision", "1", "--source", repo+"@reference/policies@handbook/policies"))

	// Materialize the delivered tree as an ordinary local directory.
	target := filepath.Join(root, "materialized")
	body(t, kcRemote(t, server.URL, "agent:operator", "dataset", "clone", "delivery", target))
	content, err := os.ReadFile(filepath.Join(target, "reference", "policies", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "policy v1\n" {
		t.Fatalf("cloned bytes: %q", content)
	}

	// A clone never overwrites: a non-empty target is refused outright.
	busy := t.TempDir()
	if err := os.WriteFile(filepath.Join(busy, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectCode(t, kcRemote(t, server.URL, "agent:operator", "dataset", "clone", "delivery", busy), "PRECONDITION_FAILED")
}

package cli

import (
	apphome "kc/home"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"testing"
)

func TestWriterPutPathHintIsStored(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		result := RunEmbeddedForTest(append([]string{"--home", dir}, args...), nil)
		if result.Status != 0 {
			t.Fatal(result.Stdout)
		}
	}
	core := "kr://acme/public/core"
	run("local", "init")
	coreDSN := internalLakeFSRepoDSN(t)
	run("local", "repository", "attach", "--repo", core, "--dsn", coreDSN)
	run("attach", "--repo", core, "--dsn", coreDSN)
	run("writer", "put", "urn:kc:object:schema/policy.body", "--repo", core, "--command-id", "schema-body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{}}}`)
	run("writer", "put", "urn:kc:object:policy/listed", "--repo", core, "--command-id", "listed",
		"--schema-ref", "schema/policy.body", "--path-hint", "policies/listed.yaml",
		"--value", `{"body":"listed"}`)
	ws, err := apphome.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	store, ok := ws.Store.Get(kernel.RepositoryID(core))
	if !ok {
		t.Fatal("missing repository")
	}
	head, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	krepo, err := ws.Reader.Require(kernel.RepositoryID(core), kernel.ErrUsageInvalid)
	if err != nil {
		t.Fatal(err)
	}
	locator, ok := krepo.(objectmodel.UnitLocator)
	if !ok {
		t.Fatalf("repository %T cannot locate unit paths", krepo)
	}
	paths, err := locator.ObjectUnitPaths("policy/listed", head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "policies/listed.yaml" {
		t.Fatalf("writer put must store --path-hint: %q", paths)
	}
}

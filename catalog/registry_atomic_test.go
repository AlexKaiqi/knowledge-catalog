package catalog_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

func TestCatalogFailedCommitLeavesStateAndHeadUnchanged(t *testing.T) {
	for _, operation := range []string{"register", "define", "retire", "archive"} {
		t.Run(operation, func(t *testing.T) {
			registry, err := catalog.NewRegistry(t.TempDir(), "kr://atomic/catalog")
			if err != nil {
				t.Fatal(err)
			}
			cat, err := catalog.NewCatalog(snapshot.NewRegistry(), registry)
			if err != nil {
				t.Fatal(err)
			}
			if err := cat.RegisterRepository("kr://atomic/source"); err != nil {
				t.Fatal(err)
			}
			if _, err := cat.DefineWorkspace("task", 1, []catalog.WorkspaceSource{{Repository: "kr://atomic/source", Selector: snapshot.DefaultRef}}); err != nil {
				t.Fatal(err)
			}
			before := catalog.NormalizeCatalogState(cat.DumpState())
			head, err := registry.Head()
			if err != nil {
				t.Fatal(err)
			}
			mutate := func() error {
				switch operation {
				case "register":
					return cat.RegisterRepository("kr://atomic/other")
				case "define":
					_, err := cat.DefineWorkspace("task", 2, []catalog.WorkspaceSource{{Repository: "kr://atomic/source", Selector: "refs/heads/next"}})
					return err
				case "retire":
					return cat.RetireWorkspace("task")
				default:
					return cat.Archive()
				}
			}
			lock := filepath.Join(registry.RootDir(), ".git", "refs", "heads", "main.lock")
			if err := os.WriteFile(lock, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := mutate(); err == nil {
				t.Fatal("blocked ref update succeeded")
			}
			if got := catalog.NormalizeCatalogState(cat.DumpState()); !reflect.DeepEqual(before, got) {
				t.Fatalf("failed commit changed visible state: before=%#v after=%#v", before, got)
			}
			if got, err := registry.Head(); err != nil || got != head {
				t.Fatalf("failed commit moved HEAD: %s %v", got, err)
			}
			if err := os.Remove(lock); err != nil {
				t.Fatal(err)
			}
			if err := mutate(); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			reopened, err := catalog.NewRegistry(registry.RootDir(), registry.CatalogID())
			if err != nil {
				t.Fatal(err)
			}
			durable, err := reopened.Load()
			if err != nil || !reflect.DeepEqual(catalog.NormalizeCatalogState(cat.DumpState()), catalog.NormalizeCatalogState(durable)) {
				t.Fatalf("retry did not persist visible state: %#v %v", durable, err)
			}
		})
	}
}

func TestCatalogInstancesSharingRegistryDoNotOverwriteEachOther(t *testing.T) {
	registry, err := catalog.NewRegistry(t.TempDir(), "kr://shared/catalog")
	if err != nil {
		t.Fatal(err)
	}
	first := catalogFromRegistry(t, registry)
	second := catalogFromRegistry(t, registry)
	if err := first.RegisterRepository("kr://shared/first"); err != nil {
		t.Fatal(err)
	}
	if err := second.RegisterRepository("kr://shared/second"); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("stale Catalog sharing Registry overwrote state: %v", err)
	}
	if len(second.Repositories()) != 0 {
		t.Fatal("rejected Catalog leaked candidate state")
	}
	state, err := registry.Load()
	if err != nil || len(state.Repositories) != 1 || state.Repositories[0] != "kr://shared/first" {
		t.Fatalf("first accepted state lost: %#v %v", state, err)
	}
}

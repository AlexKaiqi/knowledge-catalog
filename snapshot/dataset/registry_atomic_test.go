package dataset_test

import (
	"errors"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestCatalogFailedCommitLeavesStateAndHeadUnchanged(t *testing.T) {
	for _, operation := range []string{"register", "define", "retire", "unregister"} {
		t.Run(operation, func(t *testing.T) {
			authority, registry := createCatalogRegistry(t, "kr://atomic/catalog")
			cat := catalogFromRegistry(t, registry, "kr://atomic/source")
			if err := cat.RegisterRepository("kr://atomic/source"); err != nil {
				t.Fatal(err)
			}
			if _, err := cat.DefineDataset("task", 1, []catalog.DatasetSource{{Repository: "kr://atomic/source", Selector: snapshot.DefaultRef}}); err != nil {
				t.Fatal(err)
			}
			before := catalog.NormalizeState(cat.DumpState())
			head, err := registry.Head()
			if err != nil {
				t.Fatal(err)
			}
			mutate := func() error {
				switch operation {
				case "register":
					return cat.RegisterRepository("kr://atomic/other")
				case "define":
					_, err := cat.DefineDataset("task", 2, []catalog.DatasetSource{{Repository: "kr://atomic/source", Selector: snapshot.DefaultRef}})
					return err
				case "retire":
					return cat.RetireDataset("task")
				default:
					return cat.UnregisterRepository("kr://atomic/source")
				}
			}
			authority.applyErr = errors.New("authority rejected candidate")
			if err := mutate(); err == nil {
				t.Fatal("rejected authority update succeeded")
			}
			if got := catalog.NormalizeState(cat.DumpState()); !reflect.DeepEqual(before, got) {
				t.Fatalf("failed commit changed visible state: before=%#v after=%#v", before, got)
			}
			if got, err := registry.Head(); err != nil || got != head {
				t.Fatalf("failed commit moved HEAD: %s %v", got, err)
			}
			authority.applyErr = nil
			if err := mutate(); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			reopened, err := catalog.OpenSnapshotRegistry(authority, "")
			if err != nil {
				t.Fatal(err)
			}
			durable, err := reopened.Load()
			if err != nil || !reflect.DeepEqual(catalog.NormalizeState(cat.DumpState()), catalog.NormalizeState(durable)) {
				t.Fatalf("retry did not persist visible state: %#v %v", durable, err)
			}
		})
	}
}

func TestCatalogInstancesSharingRegistryDoNotOverwriteEachOther(t *testing.T) {
	_, registry := createCatalogRegistry(t, "kr://shared/catalog")
	first := catalogFromRegistry(t, registry)
	second := catalogFromRegistry(t, registry)
	if err := first.RegisterRepository("kr://shared/first"); err != nil {
		t.Fatal(err)
	}
	if err := second.RegisterRepository("kr://shared/second"); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("stale Service sharing Registry overwrote state: %v", err)
	}
	if len(second.Repositories()) != 0 {
		t.Fatal("rejected Service leaked candidate state")
	}
	state, err := registry.Load()
	if err != nil || len(state.Repositories) != 1 || state.Repositories[0] != "kr://shared/first" {
		t.Fatalf("first accepted state lost: %#v %v", state, err)
	}
}

package dataset_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestSnapshotCatalogLifecycleNoOpChecksAuthority(t *testing.T) {
	for _, operation := range []string{"register", "retire"} {
		t.Run(operation, func(t *testing.T) {
			authority, registry := createCatalogRegistry(t, "kr://no-op/catalog")
			cat := catalogFromRegistry(t, registry, "kr://no-op/source")
			if err := cat.RegisterRepository("kr://no-op/source"); err != nil {
				t.Fatal(err)
			}
			if _, err := cat.DefineKnowledgeSet("task", 1, []catalog.KnowledgeSetSource{{Repository: "kr://no-op/source", Selector: snapshot.DefaultRef}}); err != nil {
				t.Fatal(err)
			}
			if err := cat.RetireKnowledgeSet("task"); err != nil {
				t.Fatal(err)
			}
			noOp := func(target catalog.Service) error {
				switch operation {
				case "register":
					return target.RegisterRepository("kr://no-op/source")
				case "retire":
					return target.RetireKnowledgeSet("task")
				default:
					return target.RetireKnowledgeSet("task")
				}
			}
			authorityHead := func() string {
				t.Helper()
				head, err := authority.Head(snapshot.DefaultRef)
				if err != nil {
					t.Fatal(err)
				}
				return string(head)
			}
			before := catalog.NormalizeState(cat.DumpState())
			head := authorityHead()
			if err := noOp(cat); err != nil {
				t.Fatalf("current no-op: %v", err)
			}
			if got := authorityHead(); got != head {
				t.Fatal("current no-op created an authority commit")
			}
			if err := noOp(cat.ReadView(nil)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Errorf("read view accepted lifecycle no-op: %v", err)
			}

			other, err := catalog.OpenSnapshotRegistry(authority, "")
			if err != nil {
				t.Fatal(err)
			}
			advanced, err := other.Load()
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the persistence boundary directly, including an already
			// archived Service: this handle must not trust its cached lifecycle.
			advanced.Repositories = append(advanced.Repositories, "kr://no-op/other")
			if err := other.Save(advanced, "independent authority update", "", "", ""); err != nil {
				t.Fatal(err)
			}
			accepted := authorityHead()
			if accepted == head {
				t.Fatal("second Registry did not advance authority")
			}
			if err := noOp(cat); kernel.CodeOf(err) != kernel.ErrNonFastForward {
				t.Errorf("stale lifecycle no-op reported success: %v", err)
			}
			if got := catalog.NormalizeState(cat.DumpState()); !reflect.DeepEqual(got, before) {
				t.Fatalf("stale no-op changed visible state: %#v", got)
			}
			if got, err := registry.Head(); err != nil || got != head {
				t.Fatalf("stale no-op moved its accepted HEAD: %s %v", got, err)
			}
			if got := authorityHead(); got != accepted {
				t.Fatal("stale no-op changed authority")
			}
			reopened, err := catalog.OpenSnapshotRegistry(authority, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := noOp(catalogFromRegistry(t, reopened)); err != nil {
				t.Fatalf("no-op after explicit reopen: %v", err)
			}
			if got := authorityHead(); got != accepted {
				t.Fatal("no-op after reopen created an authority commit")
			}
		})
	}
}

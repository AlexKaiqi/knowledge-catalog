package catalog_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

func TestRemoteCatalogLifecycleNoOpChecksAuthority(t *testing.T) {
	for _, operation := range []string{"register", "retire", "archive"} {
		t.Run(operation, func(t *testing.T) {
			remote := bareCatalogRemote(t)
			registry, err := catalog.CreateRemoteRegistry(t.TempDir(), "kr://no-op/catalog", remote, "")
			if err != nil {
				t.Fatal(err)
			}
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
			if operation == "archive" {
				if err := cat.Archive(); err != nil {
					t.Fatal(err)
				}
			}
			noOp := func(target *catalog.Catalog) error {
				switch operation {
				case "register":
					return target.RegisterRepository("kr://no-op/source")
				case "retire":
					return target.RetireKnowledgeSet("task")
				default:
					return target.Archive()
				}
			}
			remoteHead := func() string {
				t.Helper()
				output, err := exec.Command("git", "--git-dir", remote, "rev-parse", snapshot.DefaultRef).Output()
				if err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(string(output))
			}
			before := catalog.NormalizeCatalogState(cat.DumpState())
			head := remoteHead()
			if err := noOp(cat); err != nil {
				t.Fatalf("current no-op: %v", err)
			}
			if got := remoteHead(); got != head {
				t.Fatal("current no-op created an authority commit")
			}
			if err := noOp(cat.ReadView(nil)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Errorf("read view accepted lifecycle no-op: %v", err)
			}

			other, err := catalog.OpenRemoteRegistry(t.TempDir(), registry.CatalogID(), remote, "")
			if err != nil {
				t.Fatal(err)
			}
			advanced, err := other.Load()
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the persistence boundary directly, including an already
			// archived Catalog: this handle must not trust its cached lifecycle.
			advanced.Repositories = append(advanced.Repositories, "kr://no-op/other")
			if err := other.Save(advanced, "independent authority update", "", "", ""); err != nil {
				t.Fatal(err)
			}
			accepted := remoteHead()
			if accepted == head {
				t.Fatal("second Registry did not advance authority")
			}
			if err := noOp(cat); kernel.CodeOf(err) != kernel.ErrNonFastForward {
				t.Errorf("stale lifecycle no-op reported success: %v", err)
			}
			if got := catalog.NormalizeCatalogState(cat.DumpState()); !reflect.DeepEqual(got, before) {
				t.Fatalf("stale no-op changed visible state: %#v", got)
			}
			if got, err := registry.Head(); err != nil || got != head {
				t.Fatalf("stale no-op moved its accepted HEAD: %s %v", got, err)
			}
			if got := remoteHead(); got != accepted {
				t.Fatal("stale no-op changed authority")
			}
			reopened, err := catalog.OpenRemoteRegistry(t.TempDir(), registry.CatalogID(), remote, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := noOp(catalogFromRegistry(t, reopened)); err != nil {
				t.Fatalf("no-op after explicit reopen: %v", err)
			}
			if got := remoteHead(); got != accepted {
				t.Fatal("no-op after reopen created an authority commit")
			}
		})
	}
}

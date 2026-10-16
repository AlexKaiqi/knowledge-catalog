package dataset_test

import (
	"maps"
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

// A revision's dependency versions are opaque interpretation coordinates:
// they survive reload and replay, separate PinIDs, and never become sources
// or delivered items.
func TestDatasetDependenciesFreezeWithRevisionAndStayOutOfScope(t *testing.T) {
	s := setupFed(t)
	public := kernel.RepositoryID("kr://acme/public/core")
	group := kernel.RepositoryID("kr://acme/groups/payments")
	sources := []catalog.DatasetSource{{Repository: public, Selector: snapshot.DefaultRef}}
	plain, err := s.catalog.DefineDataset("deps", 1, sources)
	if err != nil {
		t.Fatal(err)
	}
	plainPin, err := s.catalog.ResolveDefinition(plain)
	if err != nil {
		t.Fatal(err)
	}
	groupStore, ok := s.store.Get(group)
	if !ok {
		t.Fatal("group repository is not attached")
	}
	groupAt := testkit.MustHead(t, groupStore, snapshot.DefaultRef)
	candidate, err := s.catalog.PrepareDataset("deps", 2, sources)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Dependencies = map[kernel.RepositoryID]kernel.CommitID{group: groupAt}
	accepted, err := s.catalog.PublishDataset(candidate)
	if err != nil || !maps.Equal(accepted.Dependencies, candidate.Dependencies) {
		t.Fatalf("dependency versions were not kept with the revision: %#v %v", accepted, err)
	}
	pin, err := s.catalog.ResolveDataset("deps")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(pin.Dependencies, candidate.Dependencies) || len(pin.Repositories) != 1 || pin.PinID == plainPin.PinID {
		t.Fatalf("dependency must ride the pin without joining members, and change the PinID: %#v", pin)
	}
	for _, item := range pin.Items {
		if item.Repository == group {
			t.Fatalf("dependency repository delivered files: %#v", pin.Items)
		}
	}
	reopened, err := catalog.OpenSnapshotRegistry(s.authority, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.catalog, err = catalog.NewService(s.store, reopened); err != nil {
		t.Fatal(err)
	}
	replayed, err := s.catalog.ReplayPublished(pin)
	if err != nil || !reflect.DeepEqual(replayed, pin) {
		t.Fatalf("dependency versions changed across reload and replay: %#v %v", replayed, err)
	}
	if again, err := s.catalog.ResolveDefinition(plain); err != nil || again.PinID != plainPin.PinID {
		t.Fatalf("a revision without dependencies lost its historical PinID: %#v %v", again, err)
	}
}

func TestDatasetDependenciesRejectInvalidCoordinates(t *testing.T) {
	s := setupFed(t)
	public := kernel.RepositoryID("kr://acme/public/core")
	group := kernel.RepositoryID("kr://acme/groups/payments")
	sources := []catalog.DatasetSource{{Repository: public, Selector: snapshot.DefaultRef}}
	for name, dependencies := range map[string]map[kernel.RepositoryID]kernel.CommitID{
		"empty commit":         {group: ""},
		"unknown commit":       {group: "no-such-commit"},
		"unattached":           {"kr://acme/absent": "c1"},
		"second source commit": {public: "no-such-commit"},
	} {
		t.Run(name, func(t *testing.T) {
			candidate, err := s.catalog.PrepareDataset("deps", 1, sources)
			if err != nil {
				t.Fatal(err)
			}
			candidate.Dependencies = dependencies
			if _, err := s.catalog.PublishDataset(candidate); err == nil {
				t.Fatalf("accepted invalid dependency %v", dependencies)
			}
			if _, err := s.catalog.Set("deps"); err == nil {
				t.Fatal("rejected candidate became the serving revision")
			}
		})
	}
}

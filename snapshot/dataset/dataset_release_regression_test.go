package dataset_test

import (
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
	"reflect"
	"testing"
)

func TestDatasetReleaseReplaySurvivesRepublishAndReload(t *testing.T) {
	s := setupFed(t)
	sources := []catalog.DatasetSource{{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef, Path: catalog.MountPath("old"), SubPath: "policy"}}
	if _, err := s.catalog.DefineDataset("release", 1, sources); err != nil {
		t.Fatal(err)
	}
	pin, err := s.catalog.ResolveDataset("release")
	if err != nil {
		t.Fatal(err)
	}
	sources[0].Path = catalog.MountPath("new")
	sources[0].SubPath = ""
	if _, err := s.catalog.DefineDataset("release", 2, sources); err != nil {
		t.Fatal(err)
	}
	reopenedRegistry, err := catalog.OpenSnapshotRegistry(s.authority, "")
	if err != nil {
		t.Fatal(err)
	}
	s.catalog, err = catalog.NewService(s.store, reopenedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.catalog.ReplayPublished(pin)
	if err != nil || !reflect.DeepEqual(replayed, pin) {
		t.Fatalf("historical release changed: %#v %v", replayed, err)
	}
	latest, _ := s.catalog.ResolveDataset("release")
	if latest.Revision != 2 || latest.Items[0].Target != "new" {
		t.Fatal(latest)
	}
	for _, mutate := range []func(*catalog.ResolvedDataset){
		func(p *catalog.ResolvedDataset) { p.Revision = 3 },
		func(p *catalog.ResolvedDataset) { p.Ref = "v999" },
		func(p *catalog.ResolvedDataset) { p.Items[0].Prefix = "" },
		func(p *catalog.ResolvedDataset) {
			p.Repositories["kr://acme/public/core"] = "unpublished"
			p.PinID = catalog.HashResolved(p.SetID, sources, p.Repositories)
		},
	} {
		// Scope forgery is against v1, whose path is actually restricted.
		copy, _ := s.catalog.ReplayPublished(pin)
		mutate(&copy)
		if _, err := s.catalog.ReplayPublished(copy); err == nil {
			t.Fatalf("accepted forged release: %#v", copy)
		}
	}
	if err := s.catalog.RetireDataset("release"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.ReplayPublished(pin); err == nil {
		t.Fatal("retired pin replayed")
	}
}

func TestDatasetRegressionPublishedRevisionCannotBeReplaced(t *testing.T) {
	s := setupFed(t)
	sources := []catalog.DatasetSource{{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef, Path: catalog.MountPath("docs"), SubPath: "policy"}}
	if _, err := s.catalog.DefineDataset("fixed", 2, sources); err != nil {
		t.Fatal(err)
	}
	sources[0].SubPath = ""
	if _, err := s.catalog.DefineDataset("fixed", 2, sources); err == nil {
		t.Errorf("v2 was replaced with broader file scope")
	}
	if _, err := s.catalog.DefineDataset("fixed", 1, sources); err == nil {
		pin, _ := s.catalog.ResolveDataset("fixed")
		t.Errorf("older revision accepted as latest: %s", pin.Ref)
	}
}

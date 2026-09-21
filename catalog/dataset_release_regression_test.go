package catalog_test

import (
	"kc/catalog"
	"kc/snapshot"
	"reflect"
	"testing"
)

func TestDatasetReleaseReplaySurvivesRepublishAndReload(t *testing.T) {
	s := setupFed(t)
	sources := []catalog.KnowledgeSetSource{{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef, Path: catalog.MountPath("old"), SubPath: "policy"}}
	if _, err := s.catalog.DefineKnowledgeSet("release", 1, sources); err != nil {
		t.Fatal(err)
	}
	pin, err := s.catalog.ResolveKnowledgeSet("release")
	if err != nil {
		t.Fatal(err)
	}
	sources[0].Path = catalog.MountPath("new")
	sources[0].SubPath = ""
	if _, err := s.catalog.DefineKnowledgeSet("release", 2, sources); err != nil {
		t.Fatal(err)
	}
	reopenedRegistry, err := catalog.NewRegistry(s.registry.RootDir(), s.registry.CatalogID())
	if err != nil {
		t.Fatal(err)
	}
	s.catalog, err = catalog.NewCatalog(s.store, reopenedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.catalog.ReplayPublished(pin)
	if err != nil || !reflect.DeepEqual(replayed, pin) {
		t.Fatalf("historical release changed: %#v %v", replayed, err)
	}
	latest, _ := s.catalog.ResolveKnowledgeSet("release")
	if latest.Revision != 2 || latest.Items[0].Target != "new" {
		t.Fatal(latest)
	}
	for _, mutate := range []func(*catalog.ResolvedKnowledgeSet){
		func(p *catalog.ResolvedKnowledgeSet) { p.Revision = 3 },
		func(p *catalog.ResolvedKnowledgeSet) { p.Ref = "v999" },
		func(p *catalog.ResolvedKnowledgeSet) { p.Items[0].Prefix = "" },
		func(p *catalog.ResolvedKnowledgeSet) {
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
	if err := s.catalog.RetireKnowledgeSet("release"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.ReplayPublished(pin); err == nil {
		t.Fatal("retired pin replayed")
	}
}

func TestDatasetRegressionPublishedRevisionCannotBeReplaced(t *testing.T) {
	s := setupFed(t)
	sources := []catalog.KnowledgeSetSource{{Repository: "kr://acme/public/core", Selector: snapshot.DefaultRef, Path: catalog.MountPath("docs"), SubPath: "policy"}}
	if _, err := s.catalog.DefineKnowledgeSet("fixed", 2, sources); err != nil {
		t.Fatal(err)
	}
	sources[0].SubPath = ""
	if _, err := s.catalog.DefineKnowledgeSet("fixed", 2, sources); err == nil {
		t.Errorf("v2 was replaced with broader file scope")
	}
	if _, err := s.catalog.DefineKnowledgeSet("fixed", 1, sources); err == nil {
		pin, _ := s.catalog.ResolveKnowledgeSet("fixed")
		t.Errorf("older revision accepted as latest: %s", pin.Ref)
	}
}

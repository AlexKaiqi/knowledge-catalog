package catalog_test

import (
	"reflect"
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

func TestCatalogReadViewKeepsAcceptedStateAndCannotPersist(t *testing.T) {
	registry, err := catalog.NewRegistry(t.TempDir(), "kr://read-view/catalog")
	if err != nil {
		t.Fatal(err)
	}
	original := catalogFromRegistry(t, registry, "kr://read-view/source")
	if err := original.RegisterRepository("kr://read-view/source"); err != nil {
		t.Fatal(err)
	}
	if _, err := original.DefineKnowledgeSet("task", 1, []catalog.KnowledgeSetSource{{Repository: "kr://read-view/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	accepted := catalog.NormalizeCatalogState(original.DumpState())
	view := original.ReadView(nil)
	if err := original.RegisterRepository("kr://read-view/later"); err != nil {
		t.Fatal(err)
	}
	if got := catalog.NormalizeCatalogState(view.DumpState()); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("read view moved with a later accepted state: %#v", got)
	}
	head, err := registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func() error{
		"register": func() error { return view.RegisterRepository("kr://read-view/forbidden") },
		"define": func() error {
			_, err := view.DefineKnowledgeSet("task", 2, accepted.KnowledgeSets[0].Sources)
			return err
		},
		"retire":  func() error { return view.RetireKnowledgeSet("task") },
		"archive": view.Archive,
		"create":  view.RecordCreated,
	} {
		if err := mutate(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Errorf("%s on read view: %v", name, err)
		}
	}
	if got, err := registry.Head(); err != nil || got != head {
		t.Fatalf("read view changed authority: %s %v", got, err)
	}
	if got := catalog.NormalizeCatalogState(view.DumpState()); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("rejected mutation changed read view: %#v", got)
	}
	if original.HasRepository("kr://read-view/forbidden") {
		t.Fatal("read view changed the process Catalog")
	}
}

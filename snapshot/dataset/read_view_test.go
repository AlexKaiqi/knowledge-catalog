package dataset_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

// A request view shares the accepted Registry state, so its publication is
// immediately visible to the process Service, while its stamp stays private.
func TestCatalogRequestViewSharesStateButKeepsItsStamp(t *testing.T) {
	_, registry := createCatalogRegistry(t, "kr://request-view/catalog")
	original := catalogFromRegistry(t, registry, "kr://request-view/source")
	original.SetStamp("shared", "shared-request", "shared-rule")
	view := original.RequestView(nil, "alice", "req-1", "rule-1")
	if err := view.RegisterRepository("kr://request-view/source"); err != nil {
		t.Fatal(err)
	}
	if !original.HasRepository("kr://request-view/source") {
		t.Fatal("request view publication is not visible to the process Service")
	}
	if err := original.RegisterRepository("kr://request-view/later"); err != nil {
		t.Fatal(err)
	}
	if !view.HasRepository("kr://request-view/later") {
		t.Fatal("request view missed a later accepted state")
	}
	history, err := original.Log(catalog.LogQuery{Limit: 2})
	if err != nil || len(history.Commits) != 2 {
		t.Fatalf("history = %#v, error = %v", history, err)
	}
	later, first := history.Commits[0], history.Commits[1]
	if first.Author != "alice" || first.RequestID != "req-1" || first.RuleID != "rule-1" {
		t.Fatalf("request view commit carries another stamp: %#v", first)
	}
	if later.Author != "shared" || later.RequestID != "shared-request" || later.RuleID != "shared-rule" {
		t.Fatalf("request view rewrote the process Service stamp: %#v", later)
	}
	frozen := original.ReadView(nil).RequestView(nil, "alice", "req-2", "")
	if err := frozen.RegisterRepository("kr://request-view/forbidden"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("request view over a read view persisted: %v", err)
	}
}

func TestCatalogReadViewKeepsAcceptedStateAndCannotPersist(t *testing.T) {
	_, registry := createCatalogRegistry(t, "kr://read-view/catalog")
	original := catalogFromRegistry(t, registry, "kr://read-view/source")
	if err := original.RegisterRepository("kr://read-view/source"); err != nil {
		t.Fatal(err)
	}
	if _, err := original.DefineDataset("task", 1, []catalog.DatasetSource{{Repository: "kr://read-view/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	accepted := catalog.NormalizeState(original.DumpState())
	view := original.ReadView(nil)
	if err := original.RegisterRepository("kr://read-view/later"); err != nil {
		t.Fatal(err)
	}
	if got := catalog.NormalizeState(view.DumpState()); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("read view moved with a later accepted state: %#v", got)
	}
	head, err := registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func() error{
		"register": func() error { return view.RegisterRepository("kr://read-view/forbidden") },
		"define": func() error {
			_, err := view.DefineDataset("task", 2, accepted.Datasets[0].Sources)
			return err
		},
		"retire": func() error { return view.RetireDataset("task") },
		"create": view.RecordCreated,
	} {
		if err := mutate(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Errorf("%s on read view: %v", name, err)
		}
	}
	if got, err := registry.Head(); err != nil || got != head {
		t.Fatalf("read view changed authority: %s %v", got, err)
	}
	if got := catalog.NormalizeState(view.DumpState()); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("rejected mutation changed read view: %#v", got)
	}
	if original.HasRepository("kr://read-view/forbidden") {
		t.Fatal("read view changed the process Service")
	}
}

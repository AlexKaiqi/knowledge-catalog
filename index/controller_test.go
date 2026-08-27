package index_test

import (
	"path/filepath"
	"testing"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
)

func TestProjectionControllerDesireIsDurableAndCoalescesWithoutHydration(t *testing.T) {
	lookups := 0
	path := filepath.Join(t.TempDir(), "controller.db")
	store := index.NewTargetStore(path)
	controller, err := index.NewController(nil, store, func(kernel.RepositoryID) (knowledge.Repository, error) {
		lookups++
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Desire("kr://scale/physical", "commit-1"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Desire("kr://scale/physical", "commit-2"); err != nil {
		t.Fatal(err)
	}
	if lookups != 0 {
		t.Fatalf("receipt-path desire hydrated %d repositories", lookups)
	}
	targets, err := controller.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].DesiredCommit != "commit-2" || targets[0].Status != index.TargetPending {
		t.Fatalf("targets = %#v", targets)
	}

	reopened, err := index.NewController(nil, index.NewTargetStore(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	targets, err = reopened.Targets()
	if err != nil || len(targets) != 1 || targets[0].DesiredCommit != "commit-2" {
		t.Fatalf("reopened targets = %#v, %v", targets, err)
	}
}

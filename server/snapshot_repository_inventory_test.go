package server

import (
	"net/http"
	"reflect"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestSnapshotRepositoryInventoryPreservesExternalObjectMetadata(t *testing.T) {
	dir := t.TempDir()
	source := testkit.MakeRepository(t, "kr://inventory/source")
	store := snapshot.NewRegistry()
	if err := store.Add(source); err != nil {
		t.Fatal(err)
	}
	registry, err := dataset.CreateSnapshotRegistry(testkit.MakeTreeStore(t, "kr://fixture/registry-authority"), "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := dataset.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	opened := &Home{Dir: dir, Store: store, Reader: reader.NewReader(store),
		File:     apphome.HomeFile{Registry: apphome.HomeRegistry{ID: "catalog-A"}},
		Datasets: cat, Registry: registry,
	}
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{{ID: "inventory", Principal: "reader", Actions: []string{"inventory.read"}}}}); err != nil {
		t.Fatal(err)
	}
	facade := &httpFacade{home: dir, readHome: opened, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	// Registering the module and the external API registry must not leave two
	// owners of this exact route. ServeMux panics on duplicate registration.
	facade.registerManagementRoutes(mux)
	listed := grantRequestForTest(t, mux, "reader", http.MethodGet, "/snapshot/v1/repositories", "", http.StatusOK)
	if !reflect.DeepEqual(listed["repositories"], []any{map[string]any{"id": string(source.ID()), "schemaCount": float64(0)}}) {
		t.Fatalf("external schema metadata changed after transport relocation: %#v", listed)
	}
	inventory := grantRequestForTest(t, mux, "reader", http.MethodGet, "/snapshot/v1/inventory", "", http.StatusOK)
	if !reflect.DeepEqual(inventory["repositories"], listed["repositories"]) || !reflect.DeepEqual(inventory["datasets"], []any{}) {
		t.Fatalf("host inventory diverged from module registration and descriptions: %#v", inventory)
	}
	grantRequestForTest(t, mux, "ungranted", http.MethodGet, "/snapshot/v1/repositories", "", http.StatusForbidden)
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{{ID: "manage", Principal: "reader", Actions: []string{"repositories.manage"}}}}); err != nil {
		t.Fatal(err)
	}
	before := dataset.NormalizeState(cat.DumpState())
	grantRequestForTest(t, mux, "reader", http.MethodDelete, "/snapshot/v1/repositories/kr%3A%2F%2Fkc%2Fsystem", "", http.StatusForbidden)
	if !reflect.DeepEqual(before, dataset.NormalizeState(cat.DumpState())) {
		t.Fatal("host reserved-source policy changed membership")
	}
}

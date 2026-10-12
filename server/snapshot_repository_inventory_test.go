package server

import (
	"net/http"
	"reflect"
	"testing"

	"kc/catalog"
	apphome "kc/home"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestSnapshotRepositoryInventoryPreservesExternalKnowledgeMetadata(t *testing.T) {
	dir := t.TempDir()
	source := testkit.MakeRepository(t, "kr://inventory/source")
	store := snapshot.NewRegistry()
	if err := store.Add(source); err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.NewRegistry(t.TempDir(), "catalog-A")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository(source.ID()); err != nil {
		t.Fatal(err)
	}
	opened := &Home{Dir: dir, Store: store, Reader: reader.NewReader(store),
		File:       apphome.HomeFile{Catalogs: []apphome.HomeCatalog{{ID: "catalog-A"}}},
		Catalogs:   map[string]*catalog.Catalog{"catalog-A": cat},
		Registries: map[string]*catalog.Registry{"catalog-A": registry},
		Catalog:    cat, Registry: registry,
	}
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{{ID: "inventory", Principal: "reader", Catalog: "catalog-A", Actions: []string{"catalog.read"}}}}); err != nil {
		t.Fatal(err)
	}
	facade := &httpFacade{home: dir, readHome: opened, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	// Registering the module and the external API registry must not leave two
	// owners of this exact route. ServeMux panics on duplicate registration.
	facade.registerManagementRoutes(mux)
	listed := grantRequestForTest(t, mux, "reader", http.MethodGet, "/catalog/v1/catalogs/catalog-A/repositories", "", http.StatusOK)
	if listed["catalogId"] != "catalog-A" || !reflect.DeepEqual(listed["repositories"], []any{map[string]any{"id": string(source.ID()), "schemaCount": float64(0)}}) {
		t.Fatalf("external schema metadata changed after transport relocation: %#v", listed)
	}
	grantRequestForTest(t, mux, "reader", http.MethodGet, "/catalog/v1/catalogs/catalog-B/repositories", "", http.StatusForbidden)
}

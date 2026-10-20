package home

import (
	"slices"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

// Projection maintenance follows registration: a created-but-unattached or a
// detached source keeps its content but is not continuously re-projected, so
// removed physical resources stay removed.
func TestIndexMaintenanceInventoryFollowsRegistration(t *testing.T) {
	cfg, first, _ := managedFixture(t)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/inventory", CommandID: "inventory-create", Principal: "alice"}
	if _, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	id := kernel.RepositoryID(req.RepositoryID)
	if _, ok := ws.Store.Get(id); !ok {
		t.Fatal("created source must be connected")
	}
	if inventory := ws.indexMaintenanceInventory(); slices.Contains(inventory, id) {
		t.Fatalf("unregistered source entered maintenance: %v", inventory)
	}
	if !slices.Contains(ws.indexMaintenanceInventory(), objectmodel.SystemRepositoryID) {
		t.Fatal("registered System Repository left maintenance")
	}
	if err := ws.AttachRepository(id); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ws.indexMaintenanceInventory(), id) {
		t.Fatal("attached source must be maintained")
	}
	if err := ws.Datasets.UnregisterRepository(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Store.Get(id); !ok {
		t.Fatal("detach must keep the source connected")
	}
	if inventory := ws.indexMaintenanceInventory(); slices.Contains(inventory, id) {
		t.Fatalf("detached source stayed in maintenance: %v", inventory)
	}
}

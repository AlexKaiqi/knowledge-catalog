package home

import (
	"os/exec"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestManagedRepositoryCanAttachToAnotherCatalogWithoutProvisioning(t *testing.T) {
	cfg, first, creates := managedFixture(t)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(t.TempDir(), "second.git")
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("second Catalog: %s %v", out, err)
	}
	cfg.Catalogs = append(cfg.Catalogs, CatalogBinding{ID: "kr://managed/second", Remote: remote})
	if err := InitializeDeployment(cfg, nil); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/shared", CommandID: "shared-create", Principal: "alice"}
	grants := 0
	result, err := ws.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { grants++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	id := kernel.RepositoryID(req.RepositoryID)
	if ws.Catalogs[cfg.Catalogs[0].ID].HasRepository(id) || ws.Catalogs[cfg.Catalogs[1].ID].HasRepository(id) {
		t.Fatal("create must not register the repository in any Catalog")
	}
	for i := 0; i < 2; i++ {
		if err := ws.AttachRepository(cfg.Catalogs[0].ID, id); err != nil {
			t.Fatal(err)
		}
		if err := ws.AttachRepository(cfg.Catalogs[1].ID, id); err != nil {
			t.Fatal(err)
		}
	}
	source, ok := ws.Store.Get(id)
	if !ok {
		t.Fatal("source disappeared")
	}
	head, err := source.Head(snapshot.DefaultRef)
	if err != nil || head != result.Head || *creates != 1 || grants != 1 {
		t.Fatalf("attach provisioned, changed source or granted again: head=%s creates=%d grants=%d err=%v", head, *creates, grants, err)
	}
	if !ws.Catalogs[cfg.Catalogs[0].ID].HasRepository(id) || !ws.Catalogs[cfg.Catalogs[1].ID].HasRepository(id) {
		t.Fatal("source was not admitted to both Catalogs")
	}
}

package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestRegisterRetireArchive(t *testing.T) {
	s := setupFed(t)
	if !s.catalog.HasRepository("kr://acme/public/core") {
		t.Fatal("setup should register attached repositories")
	}
	if _, err := s.catalog.DefineWorkspace("ghost", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/unknown", Selector: "refs/heads/main"},
	}); err == nil {
		t.Fatal("unregistered source")
	}
	if _, err := s.catalog.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.catalog.RetireWorkspace("v"); err != nil {
		t.Fatal(err)
	}
	_, err := testkit.OpenWorkspace(s.catalog, "v")
	testkit.ExpectCode(t, err, kernel.ErrWorkspaceInvalid)
	if _, err := s.catalog.DefineWorkspace("v", 2, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err == nil {
		t.Fatal("retired workspace still writable")
	}

	if _, err := s.catalog.DefineWorkspace("live", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := testkit.FederatedRead(s.catalog, "live", "policy/P-103")
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}

	if err := s.catalog.Archive(); err != nil {
		t.Fatal(err)
	}
	testkit.ExpectCode(t, s.catalog.RegisterRepository("kr://acme/groups/payments"), kernel.ErrCatalogArchived)
	again, err := catalog.NewCatalog(s.store, s.registry)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Archived() {
		t.Fatal("archive must persist")
	}
}

func TestArchiveRepositoryBlocksOpenWorkspace(t *testing.T) {
	s := setupFed(t)
	if err := s.publicRepo.Archive(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := testkit.OpenWorkspace(s.catalog, "v")
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
}

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
		t.Fatal("setup should register mounted repos")
	}
	if _, err := s.catalog.DefineView("ghost", 1, []catalog.ViewSource{
		{Repository: "kr://acme/unknown", Selector: "refs/heads/main"},
	}); err == nil {
		t.Fatal("unregistered source")
	}
	if _, err := s.catalog.DefineView("v", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.catalog.RetireDefinition("v"); err != nil {
		t.Fatal(err)
	}
	_, err := s.catalog.OpenView("v")
	testkit.ExpectCode(t, err, kernel.ErrViewGenerationInvalid)
	if _, err := s.catalog.DefineView("v", 2, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err == nil {
		t.Fatal("retired view still writable")
	}

	if _, err := s.catalog.DefineView("live", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.catalog.FederatedRead("live", "policy/P-103")
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

func TestArchiveRepositoryBlocksOpenView(t *testing.T) {
	s := setupFed(t)
	if err := s.publicRepo.Archive(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.DefineView("v", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.catalog.OpenView("v")
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
}

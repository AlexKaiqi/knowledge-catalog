package catalog_test

import (
	"strings"
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
	if _, err := s.catalog.DefineKnowledgeSet("ghost", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/unknown", Selector: "refs/heads/main"},
	}); err == nil {
		t.Fatal("unregistered source")
	}
	if _, err := s.catalog.DefineKnowledgeSet("v", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.catalog.RetireKnowledgeSet("v"); err != nil {
		t.Fatal(err)
	}
	_, err := testkit.OpenKnowledgeSet(s.catalog, "v")
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeSetInvalid)
	if _, err := s.catalog.DefineKnowledgeSet("v", 2, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err == nil {
		t.Fatal("retired workspace still writable")
	}

	if _, err := s.catalog.DefineKnowledgeSet("live", 1, []catalog.KnowledgeSetSource{
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

func TestUnregisterRepositoryRemovesMember(t *testing.T) {
	s := setupFed(t)
	if !s.catalog.HasRepository("kr://acme/public/core") {
		t.Fatal("setup should register attached repositories")
	}
	if err := s.catalog.UnregisterRepository("kr://acme/public/core"); err != nil {
		t.Fatal(err)
	}
	if s.catalog.HasRepository("kr://acme/public/core") {
		t.Fatal("repository should be detached")
	}
	if err := s.catalog.UnregisterRepository("kr://acme/public/core"); kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
		t.Fatalf("second detach: %v", err)
	}
}

func TestArchiveRepositoryBlocksOpenKnowledgeSet(t *testing.T) {
	s := setupFed(t)
	if err := s.publicRepo.Archive(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.DefineKnowledgeSet("v", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := testkit.OpenKnowledgeSet(s.catalog, "v")
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
}

func TestMissingDatasetErrorNamesDataset(t *testing.T) {
	s := setupFed(t)
	_, err := s.catalog.Set("missing")
	if err == nil {
		t.Fatal("expected missing dataset")
	}
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeSetInvalid)
	if !strings.Contains(err.Error(), "dataset missing is not defined") {
		t.Fatalf("missing dataset must say dataset, got %v", err)
	}
	if strings.Contains(err.Error(), "workspace") {
		t.Fatalf("product error still says workspace: %v", err)
	}
	if err := s.catalog.RetireKnowledgeSet("missing"); err == nil {
		t.Fatal("expected missing dataset")
	} else if !strings.Contains(err.Error(), "dataset missing is not defined") {
		t.Fatalf("retire missing dataset must say dataset, got %v", err)
	}
}

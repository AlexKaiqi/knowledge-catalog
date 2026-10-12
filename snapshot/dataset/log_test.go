package dataset_test

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"

	catalog "kc/snapshot/dataset"
)

func TestCatalogLogStampsAuthor(t *testing.T) {
	s := setupFed(t)
	s.catalog.SetStamp("agent:payments", "run-42", "rule-9")
	if _, err := s.catalog.DefineKnowledgeSet("duty", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	hist := s.catalog.Log(catalog.LogQuery{Limit: 20, Dataset: "duty"})
	if len(hist.Commits) == 0 {
		t.Fatal(hist)
	}
	var saw bool
	for _, c := range hist.Commits {
		if c.Message != "dataset-define duty" {
			continue
		}
		saw = true
		if c.Author != "agent:payments" || c.RequestID != "run-42" || c.RuleID != "rule-9" {
			t.Fatalf("%#v", c)
		}
	}
	if !saw {
		t.Fatal(hist.Commits)
	}
}

// Embedding only the published ports deliberately omits the metadata capability.
type catalogHistoryOnly struct {
	snapshot.Store
	snapshot.TreeStore
	snapshot.HistoryStore
}

type registryMetadataUnavailable struct{ *catalogMem }

func (s registryMetadataUnavailable) ReadCommitMetadata(kernel.CommitID) (snapshot.CommitMetadata, error) {
	return snapshot.CommitMetadata{}, kernel.Fail(kernel.ErrVersionUnresolved, "commit metadata unavailable")
}

func TestCatalogHistoryWithoutCommitMetadataKeepsAcceptedCoordinates(t *testing.T) {
	authority := newCatalogMem("kr://audit/catalog")
	backend := catalogHistoryOnly{Store: authority, TreeStore: authority, HistoryStore: authority}
	reg, err := catalog.CreateSnapshotRegistry(backend, "")
	if err != nil {
		t.Fatal(err)
	}
	cat := catalogFromRegistry(t, reg)
	cat.SetStamp("agent:payments", "run-42", "rule-9")
	if err := cat.RegisterRepository("kr://audit/source"); err != nil {
		t.Fatal(err)
	}
	head, _ := reg.Head()
	hist := cat.Log(catalog.LogQuery{Limit: 1})
	if len(hist.Commits) != 1 || hist.Commits[0].Commit != head {
		t.Fatalf("accepted coordinate lost without optional metadata: %#v", hist)
	}
	got := hist.Commits[0]
	if got.Author != "" || got.Message != "" || got.RequestID != "" || got.RuleID != "" {
		t.Fatalf("metadata fabricated without capability: %#v", got)
	}
}

func TestCatalogHistoryDoesNotFabricateUnavailableCommitMetadata(t *testing.T) {
	authority := newCatalogMem("kr://audit/catalog")
	reg, err := catalog.CreateSnapshotRegistry(registryMetadataUnavailable{authority}, "")
	if err != nil {
		t.Fatal(err)
	}
	cat := catalogFromRegistry(t, reg)
	if got := cat.Log(catalog.LogQuery{}); len(got.Commits) != 0 {
		t.Fatalf("unavailable metadata was reported as recovered: %#v", got)
	}
}

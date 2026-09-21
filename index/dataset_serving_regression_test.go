package index_test

import (
	"context"
	"kc/internal/testkit"
	"kc/retrieval"
	"testing"
)

func TestDatasetServingBasisSurvivesSourceAdvanceAndReopen(t *testing.T) {
	repo, _, published := committedSearchablePolicy(t)
	idx := liveIndex(t)
	controller := newTestController(t, idx, repo)
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.PrepareServingBasis(context.Background(), repo, published); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.SearchAt(repo, published, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); err != nil {
		t.Fatal(err)
	}
	putAt(t, repo, published, testkit.PutEntity("policy/P-2", map[string]any{"body": "next runbook"}, ""))
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.SearchAt(repo, published, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); err != nil {
		t.Fatalf("still-published Dataset basis became unsearchable after source advance: %v", err)
	}
	reopened := liveIndex(t)
	if _, err := reopened.SearchAt(repo, published, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); err != nil {
		t.Fatalf("retained serving basis did not survive reopen: %v", err)
	}
}

package projection_test

import (
	"context"
	"kc/declarativeindex/internal/testfixture"
	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"testing"
)

func TestDatasetCandidateScopeRunsBeforeCanonicalHydration(t *testing.T) {
	repo, _, commit := testfixture.CommittedSearchablePolicy(t)
	engine := &staleCandidateEngine{candidates: []ports.CandidateRef{{Repository: repo.ID(), ObjectID: "policy/P-1", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	wrapped := &batchCountingRepository{Repository: repo}
	calls := 0
	ctx := ports.WithCandidateScope(context.Background(), "scope", func(kernel.RepositoryID, kernel.CommitID, objectmodel.ObjectID) (bool, error) {
		calls++
		return false, nil
	})
	result, err := idx.SearchAtContext(ctx, wrapped, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(result.Hits) != 0 || calls != 1 {
		t.Fatalf("scope: %#v %d %v", result, calls, err)
	}
	for _, batch := range wrapped.batches {
		for _, id := range batch {
			if id == "policy/P-1" {
				t.Fatal("out-of-scope body was hydrated")
			}
		}
	}
}

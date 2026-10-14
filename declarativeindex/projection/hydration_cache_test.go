package projection_test

import (
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	readcache "kc/object/cache"
	"kc/snapshot"
)

func TestSearchReusesHydrationCacheAcrossQueriesAndRejectsWrongBasis(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/public/cache-search")
	commit := testfixture.PutAt(t, repo, testkit.MustHead(t, repo, snapshot.DefaultRef), []objectmodel.Operation{
		testfixture.PolicyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "runbook alpha"}, "")[0],
	})
	engine := &staleCandidateEngine{candidates: []ports.CandidateRef{{Repository: repo.ID(), ObjectID: "policy/P-1", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	cache, err := readcache.New(readcache.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	idx.SetHydrator(cache)
	wrapped := &batchCountingRepository{Repository: repo}
	for _, query := range []string{"runbook", "alpha"} {
		result, err := idx.SearchAt(wrapped, commit, retrieval.SearchOf(retrieval.SearchMATCH(query)))
		if err != nil || len(result.Hits) != 1 {
			t.Fatalf("cached query: %+v %v", result, err)
		}
		if result.Hits[0].Object.Value.(map[string]any)["body"] != "runbook alpha" {
			t.Fatal("shared cached value was mutated")
		}
		result.Hits[0].Object.Value.(map[string]any)["body"] = "caller mutation"
	}
	objectReads := 0
	for _, batch := range wrapped.batches {
		for _, id := range batch {
			if id == "policy/P-1" {
				objectReads++
			}
		}
	}
	if objectReads != 1 {
		t.Fatalf("repeated candidates should share one authority read: %v", wrapped.batches)
	}
	engine.candidates[0].Basis = "wrong"
	if _, err := idx.SearchAt(wrapped, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cached body bypassed candidate basis validation: %v", err)
	}
}

func TestRelationsReuseSameBasisHydrationCache(t *testing.T) {
	repo, commit := testfixture.RelationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: ports.Meta{State: ports.ProjectionStateReady, Basis: commit}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), Basis: commit, ObjectID: "relation:owned"}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	defer idx.Close()
	cache, err := readcache.New(readcache.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	idx.SetHydrator(cache)
	wrapped := &batchCountingRepository{Repository: repo}
	for i := 0; i < 2; i++ {
		out, err := idx.RelationsAt(wrapped, commit, relationRequest(repo.ID()))
		if err != nil || len(out.Hits) != 1 {
			t.Fatalf("relation cache: %+v %v", out, err)
		}
	}
	if wrapped.batchCalls != 1 {
		t.Fatalf("relation candidates rehydrated %d times", wrapped.batchCalls)
	}
}

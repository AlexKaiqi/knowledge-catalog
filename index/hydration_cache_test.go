package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	readcache "kc/retrieval/cache"
	"kc/snapshot"
)

func TestSearchReusesHydrationCacheAcrossQueriesAndRejectsWrongBasis(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/cache-search")
	commit := putAt(t, repo, testkit.MustHead(t, repo, snapshot.DefaultRef), []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "runbook alpha"}, "")[0],
	})
	engine := &staleCandidateEngine{candidates: []index.CandidateRef{{Repository: repo.ID(), ObjectID: "policy/P-1", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
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
		if result.Hits[0].Knowledge.Value.(map[string]any)["body"] != "runbook alpha" {
			t.Fatal("shared cached value was mutated")
		}
		result.Hits[0].Knowledge.Value.(map[string]any)["body"] = "caller mutation"
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
	repo, commit := relationFixture(t)
	calls := []string{}
	engine := &relationEngine{meta: index.Meta{State: index.ProjectionStateReady, Basis: commit}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), Basis: commit, ObjectID: "relation:owned"}}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
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

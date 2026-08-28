package index_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

// T8 belongs to layer ③: locate candidates in a rebuildable projection, then
// hydrate every hit from Canonical at the projection basis.
func TestT8ProjectionLocateHydrateBasisLagAndRebuild(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/t8")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	c1 := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "tested runbook"}, "")[0],
		testkit.PutEntity("policy/P-2", map[string]any{"body": "another runbook"}, "")[0],
	})

	projection := liveIndex(t)
	t.Cleanup(func() { _ = projection.Close() })
	if _, err := projection.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	hits, err := projection.SearchAt(repo, c1, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 2 {
		t.Fatalf("hits=%#v err=%v", hits, err)
	}
	for _, hit := range hits.Hits {
		if hit.Knowledge.Commit != c1 || hit.Knowledge.Repository != repo.ID() {
			t.Fatalf("hit was not hydrated at projection basis: %#v", hit)
		}
	}
	desc, err := projection.Describe(repo)
	if err != nil || desc.BasisCommit != c1 || desc.LagBehindHead {
		t.Fatalf("descriptor=%#v err=%v", desc, err)
	}

	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-3", map[string]any{"body": "new runbook"}, ""))
	desc, err = projection.Describe(repo)
	if err != nil || !desc.LagBehindHead || desc.HeadCommit != c2 {
		t.Fatalf("lag descriptor=%#v err=%v", desc, err)
	}

	rebuilt := liveIndex(t)
	t.Cleanup(func() { _ = rebuilt.Close() })
	if _, err := rebuilt.Rebuild(repo, c2); err != nil {
		t.Fatal(err)
	}
	hits, err = rebuilt.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 3 {
		t.Fatalf("rebuilt hits=%#v err=%v", hits, err)
	}
}

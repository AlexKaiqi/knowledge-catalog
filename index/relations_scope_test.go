package index_test

import (
	"context"
	"fmt"
	"testing"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

func TestCrossRepositoryRelationsHydrateOnlyScopedStorageCandidates(t *testing.T) {
	repo, base := relationFixture(t)
	foreign := kernel.RepositoryID("kr://external/unattached")
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation/cross"}, Value: relationBody(foreign, "relation/cross", "Table:a")}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []bool{false, true} {
		t.Run(fmt.Sprint(allowed), func(t *testing.T) {
			calls := []string{}
			engine := &relationEngine{meta: index.Meta{Basis: commit, State: index.ProjectionStateReady}, calls: &calls, pages: [][]retrieval.RelationCandidate{{{Repository: repo.ID(), ObjectID: "relation/cross", Basis: commit}}}}
			idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			poison := &poisonAuthority{Repository: repo, batch: relationBatch(t, repo), calls: &calls, allowed: map[knowledge.ObjectID]bool{"relation/cross": allowed}}
			ctx := index.WithCandidateScope(context.Background(), "scope", func(id kernel.RepositoryID, at kernel.CommitID, object knowledge.ObjectID) (bool, error) {
				if id != repo.ID() || at != commit || object != "relation/cross" {
					t.Fatalf("scope must be checked at relation storage basis: %s %s %s", id, at, object)
				}
				return allowed, nil
			})
			page, err := idx.RelationsAtContext(ctx, poison, commit, relationRequest(foreign))
			if err != nil {
				t.Fatal(err)
			}
			if !allowed {
				if len(page.Hits) != 0 || fmt.Sprint(calls) != "[retrieve]" {
					t.Fatalf("out-of-scope relation was hydrated: %#v %v", page, calls)
				}
			} else if len(page.Hits) != 1 || page.Hits[0].Repository != repo.ID() || page.Hits[0].Commit != commit || page.Hits[0].Relation.Endpoints[0].ObjectRef.Repository != foreign || fmt.Sprint(calls) != "[retrieve read-many]" {
				t.Fatalf("cross-reference changed hydrate authority: %#v %v", page, calls)
			}
		})
	}
}

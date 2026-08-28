package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/reader"
	"kc/retrieval"
	"kc/snapshot"
)

type panicScannerRepository struct {
	*testkit.KnowledgeRepository
}

func (*panicScannerRepository) ScanSnapshotPage(kernel.CommitID, knowledgemaintenance.ScanRequest) (knowledgemaintenance.ScanPage, error) {
	panic("consumer path called maintenance scanner")
}

func TestConsumerReadSchemaSearchAndRelationsNeverCallMaintenanceScanner(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/no-consumer-scan")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/a"},
				Value: map[string]any{"body": "runbook"}, SchemaRef: "schema/policy.body"},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation:owned"},
				Value: relationBody(repo.ID(), "relation:owned", "Table:a")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &supersetEngine{ids: []knowledge.ObjectID{"policy/a"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	if _, err := idx.Ensure(repo, commit); err != nil {
		t.Fatal(err)
	}
	poison := &panicScannerRepository{KnowledgeRepository: repo}
	if _, err := poison.Read("policy/a", commit); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.DescribeRepoSchema(poison, commit, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.SearchAt(poison, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); err != nil {
		t.Fatal(err)
	}

	calls := []string{}
	relationProvider := &relationEngine{
		meta: index.Meta{Basis: commit, State: index.ProjectionStateReady, Generation: "g1"}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{
			Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit,
		}}},
	}
	relations := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return relationProvider, nil })
	if _, err := relations.RelationsAt(poison, commit, relationRequest(repo.ID())); err != nil {
		t.Fatal(err)
	}
}

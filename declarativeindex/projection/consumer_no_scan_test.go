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
	objectmaintenance "kc/object/maintenance"
	"kc/object/reader"
	"kc/snapshot"
)

type panicScannerRepository struct {
	*testkit.ObjectRepository
}

func (*panicScannerRepository) ScanSnapshotPage(kernel.CommitID, objectmaintenance.ScanRequest) (objectmaintenance.ScanPage, error) {
	panic("consumer path called maintenance scanner")
}

func TestConsumerReadSchemaSearchAndRelationsNeverCallMaintenanceScanner(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/no-consumer-scan")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy.body"}, Value: map[string]any{
				"entity": "Policy", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/a"},
				Value: map[string]any{"body": "runbook"}, SchemaRef: "schema/policy.body"},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation:owned"},
				Value: testfixture.RelationBody(repo.ID(), "relation:owned", "Table:a")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &staleCandidateEngine{candidates: []ports.CandidateRef{{ObjectID: "policy/a", Basis: commit}}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	if _, err := idx.Ensure(repo, commit); err != nil {
		t.Fatal(err)
	}
	poison := &panicScannerRepository{ObjectRepository: repo}
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
		meta: ports.Meta{Basis: commit, State: ports.ProjectionStateReady, Generation: "g1"}, calls: &calls,
		pages: [][]retrieval.RelationCandidate{{{
			Repository: repo.ID(), ObjectID: "relation:owned", Basis: commit,
		}}},
	}
	relations := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return relationProvider, nil })
	if _, err := relations.RelationsAt(poison, commit, relationRequest(repo.ID())); err != nil {
		t.Fatal(err)
	}
}

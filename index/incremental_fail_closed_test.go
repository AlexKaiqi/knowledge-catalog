package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
)

type failingIncrementalRepository struct {
	*testkit.KnowledgeRepository
	scans int
}

func (r *failingIncrementalRepository) FastChangedObjectIDs(kernel.CommitID, kernel.CommitID) ([]knowledge.ObjectID, error) {
	return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native change lookup failed")
}

func (r *failingIncrementalRepository) ScanSnapshotPage(kernel.CommitID, knowledgemaintenance.ScanRequest) (knowledgemaintenance.ScanPage, error) {
	r.scans++
	return knowledgemaintenance.ScanPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "incremental failure must not rebuild")
}

func TestEnsureFailsClosedWhenIncrementalChangeLookupFails(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://index/fail-closed")
	first := putAt(t, setup.Repo, setup.RootCommitID,
		testkit.PutEntity("policy/A", map[string]any{"version": 1}, ""))
	engine := &staleCandidateEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) {
		return engine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Ensure(setup.Repo, first); err != nil {
		t.Fatal(err)
	}
	second := putAt(t, setup.Repo, first,
		testkit.PutEntity("policy/A", map[string]any{"version": 2}, ""))
	poison := &failingIncrementalRepository{KnowledgeRepository: setup.Repo}
	_, err := idx.Ensure(poison, second)
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("incremental capability failure must be preserved, got %v", err)
	}
	if poison.scans != 0 {
		t.Fatalf("incremental failure triggered %d full rebuild scans", poison.scans)
	}
}

package projection_test

import (
	"context"
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"testing"

	index "kc/declarativeindex/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
)

type failingIncrementalRepository struct {
	*testkit.ObjectRepository
	scans int
}

func (r *failingIncrementalRepository) FastChangedObjectIDs(kernel.CommitID, kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native change lookup failed")
}

func (r *failingIncrementalRepository) ScanSnapshotPage(kernel.CommitID, objectmaintenance.ScanRequest) (objectmaintenance.ScanPage, error) {
	r.scans++
	return objectmaintenance.ScanPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "incremental failure must not rebuild")
}

func TestEnsureFailsClosedWhenIncrementalChangeLookupFails(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://index/fail-closed")
	first := testfixture.PutAt(t, setup.Repo, setup.RootCommitID,
		testkit.PutEntity("policy/A", map[string]any{"version": 1}, ""))
	engine := &staleCandidateEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) {
		return engine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Ensure(context.Background(), setup.Repo, first); err != nil {
		t.Fatal(err)
	}
	second := testfixture.PutAt(t, setup.Repo, first,
		testkit.PutEntity("policy/A", map[string]any{"version": 2}, ""))
	poison := &failingIncrementalRepository{ObjectRepository: setup.Repo}
	_, err := idx.Ensure(context.Background(), poison, second)
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("incremental capability failure must be preserved, got %v", err)
	}
	if poison.scans != 0 {
		t.Fatalf("incremental failure triggered %d full rebuild scans", poison.scans)
	}
}

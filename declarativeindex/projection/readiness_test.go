package projection_test

import (
	ports "kc/discovery/projection"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type readinessProjection struct {
	staleCandidateEngine
	countCalls int
}

func (*readinessProjection) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	panic("readiness must not probe a query")
}
func (*readinessProjection) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	panic("readiness must not retrieve candidates")
}
func (e *readinessProjection) Count() (int, error) { e.countCalls++; return 0, nil }

type readinessFixedRepository struct{ *testkit.KnowledgeRepository }

func (*readinessFixedRepository) Head(string) (kernel.CommitID, error) {
	panic("fixed readiness must not follow HEAD")
}
func (*readinessFixedRepository) Read(objectmodel.ObjectID, kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	panic("readiness must not hydrate objects")
}

func TestCheckSearchProjectionAtRequiresFixedBasisWithoutSearchOrHead(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://readiness/fixed")
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	eng := &readinessProjection{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return eng, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	eng.countCalls = 0
	fixed := &readinessFixedRepository{repo}
	meta, err := idx.CheckSearchProjectionAt(fixed, head)
	if err != nil || meta.State != ports.ProjectionStateReady || meta.Basis != head {
		t.Fatalf("fixed readiness: %+v, %v", meta, err)
	}
	if eng.countCalls != 0 {
		t.Fatal("status counted search candidates")
	}
	if _, err := idx.CheckSearchProjectionAt(fixed, ""); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("implicit HEAD accepted: %v", err)
	}
}

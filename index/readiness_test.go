package index_test

import (
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
	"kc/snapshot"
)

type readinessProjection struct {
	staleCandidateEngine
	countCalls int
}

func (*readinessProjection) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	panic("readiness must not probe a query")
}
func (*readinessProjection) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	panic("readiness must not retrieve candidates")
}
func (e *readinessProjection) Count() (int, error) { e.countCalls++; return 0, nil }

type readinessFixedRepository struct{ *testkit.KnowledgeRepository }

func (*readinessFixedRepository) Head(string) (kernel.CommitID, error) {
	panic("fixed readiness must not follow HEAD")
}
func (*readinessFixedRepository) Read(knowledge.ObjectID, kernel.CommitID) (knowledge.KnowledgeValue, error) {
	panic("readiness must not hydrate objects")
}

func TestCheckSearchProjectionAtRequiresFixedBasisWithoutSearchOrHead(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://readiness/fixed")
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	eng := &readinessProjection{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (index.Engine, error) { return eng, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	eng.countCalls = 0
	fixed := &readinessFixedRepository{repo}
	meta, err := idx.CheckSearchProjectionAt(fixed, head)
	if err != nil || meta.State != index.ProjectionStateReady || meta.Basis != head {
		t.Fatalf("fixed readiness: %+v, %v", meta, err)
	}
	if eng.countCalls != 0 {
		t.Fatal("status counted search candidates")
	}
	if _, err := idx.CheckSearchProjectionAt(fixed, ""); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("implicit HEAD accepted: %v", err)
	}
}

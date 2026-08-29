package index

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

type lifecycleEngine struct {
	meta   Meta
	closed *int
}

func (e *lifecycleEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) Capability {
	return Capability{Guarantee: GuaranteeExact, Coverage: 1}
}
func (e *lifecycleEngine) Retrieve(RetrieveRequest) (CandidatePage, error) {
	return CandidatePage{Exhausted: true}, nil
}
func (e *lifecycleEngine) LoadMeta() (Meta, error)                               { return e.meta, nil }
func (e *lifecycleEngine) Rebuild([]CompiledDoc, Meta) error                     { return nil }
func (e *lifecycleEngine) Apply([]CompiledDoc, []knowledge.ObjectID, Meta) error { return nil }
func (e *lifecycleEngine) Count() (int, error)                                   { return 0, nil }
func (e *lifecycleEngine) Close() error {
	*e.closed++
	return nil
}

func TestHistoricalReadMissEngineIsReleased(t *testing.T) {
	closed := 0
	idx := NewIndexEngine("", func(_ string, id kernel.RepositoryID) (Engine, error) {
		basis := kernel.CommitID("head")
		if id != "kr://acme/public/core" {
			basis = "historic"
		}
		return &lifecycleEngine{meta: Meta{Basis: basis}, closed: &closed}, nil
	})
	eng, release, err := idx.acquireEngineForCommit("kr://acme/public/core", "historic")
	if err != nil || eng == nil {
		t.Fatalf("acquire: %v", err)
	}
	if len(idx.engs) != 1 {
		t.Fatalf("read-only historic pin must not grow the cache: %d", len(idx.engs))
	}
	release()
	if closed != 1 {
		t.Fatalf("historic engine close count=%d", closed)
	}
	_ = idx.Close()
}

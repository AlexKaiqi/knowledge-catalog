package projection

import (
	ports "kc/discovery/projection"
	"testing"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

type lifecycleEngine struct {
	meta   ports.Meta
	closed *int
}

func (e *lifecycleEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeExact, Coverage: 1}
}
func (e *lifecycleEngine) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	return ports.CandidatePage{Exhausted: true}, nil
}
func (e *lifecycleEngine) LoadMeta() (ports.Meta, error)                 { return e.meta, nil }
func (e *lifecycleEngine) Rebuild([]ports.CompiledDoc, ports.Meta) error { return nil }
func (e *lifecycleEngine) Apply([]ports.CompiledDoc, []objectmodel.ObjectID, ports.Meta) error {
	return nil
}
func (e *lifecycleEngine) Count() (int, error) { return 0, nil }
func (e *lifecycleEngine) Close() error {
	*e.closed++
	return nil
}

func TestHistoricalReadMissEngineIsReleased(t *testing.T) {
	closed := 0
	idx := NewIndexEngine("", func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		basis := kernel.CommitID("head")
		if id != "kr://acme/public/core" {
			basis = "historic"
		}
		return &lifecycleEngine{meta: ports.Meta{Basis: basis}, closed: &closed}, nil
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

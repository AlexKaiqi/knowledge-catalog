package projection

import (
	"context"
	"errors"
	"testing"

	ports "kc/discovery/projection"
	"kc/object/reader"
)

type publicationLookupEngine struct {
	lifecycleEngine
	binds    int
	evidence []byte
	bound    ports.Engine
	err      error
}

func (e *publicationLookupEngine) BindPublication(_ context.Context, raw []byte) (ports.Engine, error) {
	e.binds++
	e.evidence = append([]byte(nil), raw...)
	return e.bound, e.err
}

func TestTemporaryDatasetQueryCannotBorrowNamedPublicationReceipt(t *testing.T) {
	engine := &publicationLookupEngine{}
	scope := datasetSpace{id: "temporary-scope", basis: "fixed", pin: reader.DatasetPin{SetID: "published-label", Revision: 7, Temporary: true}}
	idx := &Index{engs: map[engineKey]ports.Engine{{repo: scope.id, commit: scope.basis}: engine}}
	calls := 0
	idx.SetDatasetPublicationLookup(func(context.Context, string, int) ([]byte, error) { calls++; return []byte("named-authority"), nil })
	actual, release, err := idx.acquireDatasetPublication(context.Background(), scope, datasetContinuation{})
	if err != nil || actual != engine || calls != 0 || engine.binds != 0 {
		t.Fatalf("temporary label selected named publication: engine=%v lookup=%d bind=%d err=%v", actual, calls, engine.binds, err)
	}
	release()
}

func TestKnownPublicationBindingFailureCannotFallBackToCachedEngine(t *testing.T) {
	failed := errors.New("known accepted generation binding is missing")
	engine := &publicationLookupEngine{err: failed}
	scope := datasetSpace{id: "accepted-scope", basis: "fixed", pin: reader.DatasetPin{SetID: "accepted", Revision: 1}}
	idx := &Index{engs: map[engineKey]ports.Engine{{repo: scope.id, commit: scope.basis}: engine}}
	idx.SetDatasetPublicationLookup(func(context.Context, string, int) ([]byte, error) { return []byte("exact accepted evidence"), nil })
	actual, _, err := idx.acquireDatasetPublication(context.Background(), scope, datasetContinuation{})
	if !errors.Is(err, failed) || actual != nil || engine.binds != 1 || string(engine.evidence) != "exact accepted evidence" {
		t.Fatalf("binding failure fell back to cached scope: actual=%v calls=%d err=%v", actual, engine.binds, err)
	}
}

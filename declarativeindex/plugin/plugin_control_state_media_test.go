package plugin_test

import (
	"context"
	"strings"
	"testing"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	objectmodel "kc/object"
)

func TestIndexControlRestoresLostPhysicalStateWithResidentServingObservation(t *testing.T) {
	f := newControlRecoveryFixture(t)
	source := f.base.setups[0]
	at, _ := controlBoundJob(t, f)
	runtime := f.open(t, source.RepositoryID)
	f.publish(t, runtime, "resident-state", source.RepositoryID, at)
	lookup := &controlStateLookup{status: "observed-state", revision: "r1"}
	runtime.plugins.SetObservationSource(controlObservationSource(lookup))
	if err := runtime.plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertControlState(t, f, runtime, lookup, at, "observed-state", "a-owner", "r1")
	revision, ready := runtime.plugins.StateView(source.RepositoryID, at)
	if !ready || revision == "" || f.stateFiles(t) == 0 {
		t.Fatal("actual initial State projection and observation were not resident")
	}
	evidence := f.accepted(t)
	f.base.pool.mu.Lock()
	discarded := 0
	for key, store := range f.base.pool.stores {
		if !strings.Contains(key, string(source.RepositoryID)+"@") || !strings.HasSuffix(key, "@"+string(at)+"#state") {
			continue
		}
		store.mu.Lock()
		store.meta = index.Meta{}
		store.docs = map[objectmodel.ObjectID]index.CompiledDoc{}
		store.mu.Unlock()
		discarded++
	}
	f.base.pool.mu.Unlock()
	if discarded != 1 || f.stateFiles(t) == 0 {
		t.Fatalf("fault did not lose exactly the physical State while preserving observation storage: projections=%d", discarded)
	}
	if still, ready := runtime.plugins.StateView(source.RepositoryID, at); !ready || still != revision {
		t.Fatal("fault unexpectedly deleted the resident observation or changed its revision")
	}
	repo, err := f.base.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	query := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "observed-state"})
	beforeCalls := len(lookup.requests)
	if result, err := runtime.plugins.SearchStateAtRevisionContext(context.Background(), repo, at, revision, query); err == nil {
		t.Errorf("lost physical State was not an explicit query failure: %#v", result)
	}
	if len(lookup.requests) != beforeCalls {
		t.Fatal("query attempted to repair physical State through external observation")
	}
	if err := runtime.plugins.CatchUp(context.Background()); err != nil {
		t.Errorf("actual maintenance did not repair lost physical State: %v", err)
	}
	assertControlState(t, f, runtime, lookup, at, "observed-state", "a-owner", "r1")
	f.assertAccepted(t, evidence)
}

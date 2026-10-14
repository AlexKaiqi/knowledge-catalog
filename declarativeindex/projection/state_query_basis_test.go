package projection

import (
	"context"
	"reflect"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"
)

type stateQueryBasisEngine struct {
	testfixture.StateEngine
	probes, retrieves int
}

func (e *stateQueryBasisEngine) Probe(clause retrieval.SearchClause, spec retrieval.AccessSpec) ports.Capability {
	e.probes++
	return e.StateEngine.Probe(clause, spec)
}
func (e *stateQueryBasisEngine) Retrieve(request ports.RetrieveRequest) (ports.CandidatePage, error) {
	e.retrieves++
	return e.StateEngine.Retrieve(request)
}

// This repairs a pre-existing mismatch at the State query boundary; moving the
// implementation did not introduce it. U5 requires the provider and resident
// observation to describe the same published basis before any candidate read.
func TestStateQueryRejectsProviderObservationMismatchBeforeCandidates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ports.Meta)
	}{
		{"revision", func(meta *ports.Meta) { meta.Revision = "another-observation" }},
		{"observation digest", func(meta *ports.Meta) { meta.ObservationDigest = "another-digest" }},
		{"declaration basis", func(meta *ports.Meta) { meta.Basis = "another-declaration" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, commit, address := testfixture.StateProjection(t)
			engine := &stateQueryBasisEngine{}
			idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{
				BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z",
			}}
			published, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
			if err != nil {
				t.Fatal(err)
			}
			request := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.runtime", Aspect: "runtime", Path: "status"}, Value: "running"})
			before, err := idx.SearchStateAtRevisionContext(context.Background(), repo, commit, published.Revision, request)
			if err != nil || before.Completeness != retrieval.CompletenessComplete || len(before.Hits) != 1 || len(before.Hits[0].Version.Observations) != 1 || before.Hits[0].Version.Observations[0].Basis.SourceRevision != "r1" {
				t.Fatalf("initial actual State query was not published at r1: result=%#v error=%v", before, err)
			}
			state := idx.states[stateStoreKey(repo.ID(), commit)]
			originalRecord, found, err := state.store.Get(address.ObjectID)
			if err != nil || !found {
				t.Fatalf("published Serving State missing before fault: found=%v error=%v", found, err)
			}
			originalMeta := engine.Meta
			engine.probes, engine.retrieves = 0, 0
			calls := lookup.Calls
			tc.change(&engine.Meta)
			rejected, err := idx.SearchStateAtRevisionContext(context.Background(), repo, commit, published.Revision, request)
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(rejected.Hits) != 0 || engine.probes != 0 || engine.retrieves != 0 {
				t.Errorf("wrong provider observation reached query/hydration: result=%#v error=%v probes=%d retrieves=%d", rejected, err, engine.probes, engine.retrieves)
			}
			currentRecord, found, readErr := state.store.Get(address.ObjectID)
			if readErr != nil || !found || !reflect.DeepEqual(currentRecord, originalRecord) || lookup.Calls != calls {
				t.Fatalf("rejected query changed or reobserved Serving State: found=%v error=%v lookups=%d want=%d", found, readErr, lookup.Calls, calls)
			}
			if revision, ok := idx.StateView(repo.ID(), commit); !ok || revision != published.Revision {
				t.Fatal("provider fault changed resident observation identity")
			}
			if head := testkit.MustHead(t, repo, snapshot.DefaultRef); head != commit {
				t.Fatal("query changed the declaration repository")
			}
			engine.Meta = originalMeta
			restored, err := idx.SearchStateAtRevisionContext(context.Background(), repo, commit, published.Revision, request)
			before.Stats, restored.Stats = retrieval.SearchExecutionStats{}, retrieval.SearchExecutionStats{}
			if err != nil || !reflect.DeepEqual(restored, before) || lookup.Calls != calls {
				t.Fatalf("restoring provider basis did not preserve the original observation: result=%#v error=%v", restored, err)
			}
		})
	}
}

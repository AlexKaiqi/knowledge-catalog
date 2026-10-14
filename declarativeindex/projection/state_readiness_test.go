package projection

import (
	"context"
	"errors"
	"os"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
)

type stateReadinessEngine struct {
	testfixture.StateEngine
	metaErr error
	reads   int
}

func (*stateReadinessEngine) ProviderID() string            { return "state-readiness" }
func (*stateReadinessEngine) ProviderRevision() string      { return "v1" }
func (*stateReadinessEngine) PhysicalDigest() kernel.Digest { return "physical-v1" }
func (e *stateReadinessEngine) LoadMetaContext(ctx context.Context) (ports.Meta, error) {
	e.reads++
	if err := ctx.Err(); err != nil {
		return ports.Meta{}, err
	}
	return e.Meta, e.metaErr
}
func (*stateReadinessEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	panic("State readiness must not probe a query")
}
func (*stateReadinessEngine) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	panic("State readiness must not retrieve candidates")
}
func (*stateReadinessEngine) Count() (int, error) {
	panic("State readiness must not count candidates")
}

func TestStateProjectionReadinessObservesPhysicalAndServingMediaWithoutRefresh(t *testing.T) {
	repo, commit, _ := testfixture.StateProjection(t)
	engine := &stateReadinessEngine{}
	opens := 0
	idx := NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) {
		opens++
		return engine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	if ready, err := idx.StateProjectionReady(context.Background(), repo, commit); err != nil || ready || opens != 0 {
		t.Fatalf("unpublished State opened a provider or appeared ready: ready=%v opens=%d error=%v", ready, opens, err)
	}
	lookup := &testfixture.StateLookup{Value: map[string]any{"status": "running"}, Basis: objectmodel.ObservationBasis{
		BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z",
	}}
	published, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
	if err != nil {
		t.Fatal(err)
	}
	original, calls := engine.Meta, lookup.Calls
	if ready, err := idx.StateProjectionReady(context.Background(), repo, commit); err != nil || !ready {
		t.Fatalf("published physical and Serving State were not ready: ready=%v error=%v", ready, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*ports.Meta)
	}{
		{"lost", func(meta *ports.Meta) { *meta = ports.Meta{} }},
		{"basis", func(meta *ports.Meta) { meta.Basis = "other" }},
		{"lifecycle", func(meta *ports.Meta) { meta.State = "" }},
		{"access", func(meta *ports.Meta) { meta.AccessDigest = "other" }},
		{"physical", func(meta *ports.Meta) { meta.PhysicalDigest = "other" }},
		{"provider revision", func(meta *ports.Meta) { meta.ProviderRevision = "other" }},
		{"revision", func(meta *ports.Meta) { meta.Revision = "other" }},
		{"observation", func(meta *ports.Meta) { meta.ObservationDigest = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine.Meta = original
			tc.change(&engine.Meta)
			if ready, err := idx.StateProjectionReady(context.Background(), repo, commit); err != nil || ready {
				t.Fatalf("incompatible physical State appeared ready: ready=%v error=%v", ready, err)
			}
			if revision, ok := idx.StateView(repo.ID(), commit); !ok || revision != published.Revision {
				t.Fatal("readiness changed the published observation identity")
			}
		})
	}
	engine.Meta = original
	problem := kernel.Fail(kernel.ErrTemporaryUnavailable, "provider metadata unavailable")
	engine.metaErr = problem
	if ready, err := idx.StateProjectionReady(context.Background(), repo, commit); ready || !errors.Is(err, problem) {
		t.Fatalf("provider read error was hidden: ready=%v error=%v", ready, err)
	}
	engine.metaErr = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reads := engine.reads
	if ready, err := idx.StateProjectionReady(ctx, repo, commit); ready || !errors.Is(err, context.Canceled) || engine.reads != reads {
		t.Fatalf("canceled readiness accessed the provider: ready=%v reads=%d error=%v", ready, engine.reads, err)
	}
	if err := os.Remove(idx.states[stateStoreKey(repo.ID(), commit)].store.file); err != nil {
		t.Fatal(err)
	}
	if ready, err := idx.StateProjectionReady(context.Background(), repo, commit); err != nil || ready {
		t.Fatalf("lost Serving medium appeared ready: ready=%v error=%v", ready, err)
	}
	if lookup.Calls != calls {
		t.Fatalf("readiness refreshed external values: lookups=%d want=%d", lookup.Calls, calls)
	}
}

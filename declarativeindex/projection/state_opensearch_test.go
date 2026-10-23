package projection_test

import (
	"context"
	"kc/declarativeindex/internal/testfixture"
	sourcebridge "kc/liveaccess/discovery"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

type liveStateLookup struct {
	status   string
	revision string
}

func (s *liveStateLookup) LookupState(context.Context, objectserving.StateLookupRequest) (objectserving.StateObservation, error) {
	return objectserving.StateObservation{
		Value: map[string]any{"status": s.status, "attempts": float64(3)},
		Basis: objectmodel.ObservationBasis{
			BindingGeneration: "scheduler-v1", Consistency: objectmodel.ObservationRepeatable,
			SourceRevision: s.revision, ObservedAt: "2026-08-27T14:00:00Z",
		},
	}, nil
}

func TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://acme/state")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/orders", AspectName: "runtime"}
	commit := testfixture.PutAt(t, repo, root, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.definition"}, Value: map[string]any{
			"entity": "Job", "aspect": "definition", "fields": map[string]any{
				"owner": map[string]any{"type": "string"},
			},
		}},
		testkit.IndexField("schema/job.definition", "owner", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
			"entity": "Job", "aspect": "runtime", "fields": map[string]any{
				"status":   map[string]any{"type": "string"},
				"attempts": map[string]any{"type": "integer"},
			},
		}},
		testkit.IndexText("schema/job.runtime", "status"),
		testkit.IndexField("schema/job.runtime", "status", "keyword"),
		testkit.IndexField("schema/job.runtime", "attempts", "number", "filter", "sort"),
		bindingfixture.PutDescriptor("resource/scheduler", bindingfixture.Descriptor("https://scheduler.example")),
		bindingfixture.Bind(address, "schema/job.runtime", "resource/scheduler"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "data"}, SchemaRef: "schema/job.definition"},
	})
	directory, opener := t.TempDir(), testfixture.LiveEngineOpener(t)
	idx := index.NewIndexEngine(directory, opener)
	t.Cleanup(func() { _ = idx.Close() })
	lookup := &liveStateLookup{status: "running", revision: "r1"}
	first, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup})
	if err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchOf(
		retrieval.SearchMATCH("running"),
		retrieval.SearchRange(retrieval.OpGTE, "attempts", "3"),
		retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/job.definition", Aspect: "definition", Path: "owner"}, Value: "data"},
	)
	result, err := idx.SearchStateAt(repo, commit, request)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("dynamic OpenSearch result: %#v %v", result, err)
	}
	if result.SearchView.IndexRevisions[repo.ID()] != first.Revision || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("SearchView/hit did not retain observation basis: %#v", result)
	}

	// Closing the actual engine owner must preserve the fixed observed values.
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	idx = index.NewIndexEngine(directory, opener)
	lookup.status, lookup.revision = "unexpected-new-value", "unexpected-r2"
	recovered, err := idx.SearchStateAtRevision(repo, commit, first.Revision, request)
	if err != nil || len(recovered.Hits) != 1 || recovered.Hits[0].Version.Observations[0].Basis.SourceRevision != "r1" {
		t.Fatalf("actual OpenSearch restart lost or refreshed the original observation: %#v %v", recovered, err)
	}

	lookup.status, lookup.revision = "stopped", "r2"
	if _, err := idx.RefreshState(context.Background(), repo, commit, sourcebridge.Source{Lookup: lookup}); err != nil {
		t.Fatal(err)
	}
	old, err := idx.SearchStateAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("running")))
	if err != nil || len(old.Hits) != 0 {
		t.Fatalf("old observation survived incremental Apply: %#v %v", old, err)
	}
	if head, err := repo.Head(snapshot.DefaultRef); err != nil || head != kernel.CommitID(commit) {
		t.Fatalf("observation refresh changed Repository HEAD: %s %v", head, err)
	}
}

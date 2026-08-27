package index_test

import (
	"context"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
	"kc/snapshot"
)

type liveStateLookup struct {
	status   string
	revision string
}

func (s *liveStateLookup) LookupState(context.Context, knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	return knowledgeserving.StateObservation{
		Value: map[string]any{"status": s.status, "attempts": float64(3)},
		Basis: knowledge.ObservationBasis{
			BindingGeneration: "scheduler-v1", Consistency: knowledge.ObservationRepeatable,
			SourceRevision: s.revision, ObservedAt: "2026-08-27T14:00:00Z",
		},
	}, nil
}

func TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/state")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Job:orders", AspectName: "runtime"}
	commit := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/job.definition"}, Value: map[string]any{
			"entity": "Job", "aspect": "definition", "fields": map[string]any{
				"owner": map[string]any{"type": "string", "access": []any{"filter"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
			"entity": "Job", "aspect": "runtime", "fields": map[string]any{
				"status":   map[string]any{"type": "string", "access": []any{"text", "filter"}},
				"attempts": map[string]any{"type": "integer", "access": []any{"filter", "sort"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "data"}, SchemaRef: "schema/job.definition"},
		{Op: knowledge.OpPut, Address: address, Value: nil, SchemaRef: "schema/job.runtime", ValueSource: &knowledge.ValueSource{
			Kind:    knowledge.ValueSourceBinding,
			Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, Runtime: "scheduler", Protocol: "resource-access/v1", Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "job.status"}}},
		}},
	})
	idx := liveIndex(t)
	lookup := &liveStateLookup{status: "running", revision: "r1"}
	first, err := idx.RefreshState(context.Background(), repo, commit, lookup, knowledgeserving.RequestContext{})
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
	if result.SearchView.ProjectionRevisions[repo.ID()] != first.Revision || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("SearchView/hit did not retain observation basis: %#v", result)
	}

	lookup.status, lookup.revision = "stopped", "r2"
	if _, err := idx.RefreshState(context.Background(), repo, commit, lookup, knowledgeserving.RequestContext{}); err != nil {
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

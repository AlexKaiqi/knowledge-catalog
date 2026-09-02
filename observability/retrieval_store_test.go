package observability_test

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

func TestRetrievalEvidenceQueryTraceAndTrainingAreRebuildable(t *testing.T) {
	store := observability.NewFileStore(t.TempDir())
	identity := observability.IdentityContext{Principal: "agent:answer", OnBehalfOf: "user:kai"}
	trace := observability.TraceContext{TraceID: "trace-search", SpanID: "span-search"}
	ref := knowledge.KnowledgeRef{Repository: "kr://acme/runbooks", Object: "runbook/refund"}
	accessID, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-08-31T00:00:00.1Z", Identity: identity, Trace: trace,
		Action: "knowledge.search", Workspace: "agent", Decision: "ALLOW", Result: "RESOLVED",
		Knowledge: []observability.KnowledgeAccess{{KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	logical := map[string]any{"clauses": []any{map[string]any{"op": "MATCH", "value": "refund timeout"}}, "limit": 10}
	candidates := []observability.RetrievalCandidate{{
		KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}, Rank: 1,
		ValueDigest: "value-1", Evidence: []observability.RetrievalLaneEvidence{{Provider: "opensearch", Lane: "lexical", Guarantee: "exact", LocalRank: 1}},
	}}
	retrievalID, err := store.RecordRetrievalReceipt(observability.RetrievalEvent{
		AccessEvidenceID: accessID, OccurredAt: "2026-08-31T00:00:00.2Z", Identity: identity, Trace: trace,
		Action: "knowledge.search", Workspace: "agent", Operator: observability.RetrievalOperatorSearch,
		LogicalRequest: logical, RequestDigest: kernel.CanonicalDigest(logical),
		SearchView:   observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{ref.Repository: "c1"}},
		Completeness: "complete", Execution: observability.RetrievalExecution{Candidates: 1, Hydrated: 1},
		Candidates: candidates, CandidateDigest: kernel.CanonicalDigest(candidates), Outcome: "COMPLETED",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, feedback := range []observability.FeedbackEvent{
		{OccurredAt: "2026-08-31T00:00:00.3Z", Identity: identity, Trace: trace, Workspace: "agent", Outcome: "answered",
			RetrievalEvidenceID: retrievalID, LabelSource: "agent", Answer: "Inspect the refund timeout.", SelectedRefs: []knowledge.KnowledgeRef{ref}},
		{OccurredAt: "2026-08-31T00:00:00.4Z", Identity: observability.IdentityContext{Principal: "user:kai"}, Trace: trace,
			Workspace: "agent", Outcome: "accepted", RetrievalEvidenceID: retrievalID, LabelSource: "user"},
	} {
		if err := store.RecordFeedback(context.Background(), feedback); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.Retrieval(observability.RetrievalQuery{TraceID: trace.TraceID, Operator: "SEARCH", Provider: "opensearch", Outcome: "COMPLETED"})
	if err != nil || len(events) != 1 || events[0].EvidenceID != retrievalID {
		t.Fatalf("retrieval events=%#v err=%v", events, err)
	}
	view, err := store.Trace(trace.TraceID)
	if err != nil || len(view.Entries) != 4 || view.Entries[1].Kind != "retrieval" {
		t.Fatalf("trace=%#v err=%v", view, err)
	}
	samples, err := store.RetrievalTrainingSamples(observability.RetrievalQuery{EvidenceID: retrievalID})
	if err != nil || len(samples) != 1 || !samples[0].TrainingEligible || samples[0].LabelStrength != "accepted-answer" {
		t.Fatalf("samples=%#v err=%v", samples, err)
	}
}

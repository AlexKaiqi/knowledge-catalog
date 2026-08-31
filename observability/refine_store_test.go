package observability_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

func TestRefineEvidenceTraceAndTrainingSamplesAreRebuildable(t *testing.T) {
	store := observability.NewFileStore(t.TempDir())
	identity := observability.IdentityContext{Principal: "agent:answer", OnBehalfOf: "user:kai"}
	trace := observability.TraceContext{TraceID: "trace-rerank", SpanID: "span-rerank"}
	ref := knowledge.KnowledgeRef{Repository: "kr://acme/runbooks", Object: "runbook/refund"}
	accessID, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-08-31T00:00:00.1Z", Identity: identity, Trace: trace,
		Action: "knowledge.rerank", Workspace: "agent", Decision: "ALLOW", Result: "RESOLVED",
		Knowledge: []observability.KnowledgeAccess{{KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"body": "check refund timeout and idempotency key"}
	refineID, err := store.RecordRefineReceipt(observability.RefineEvent{
		AccessEvidenceID: accessID, OccurredAt: "2026-08-31T00:00:00.2Z", Identity: identity, Trace: trace,
		Action: "knowledge.rerank", RequestID: "req-1", Workspace: "agent",
		SearchView:      observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/runbooks": "c1"}},
		Spec:            observability.RefineSpec{SpecRef: "urn:rank:refund", Revision: 1, Operator: "SEMANTIC_RERANK", Criterion: "refund timeout relevance"},
		CandidateDigest: "digest-1", ProjectedBytes: 128,
		Candidates: []observability.RefineCandidate{{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}, Value: value,
			ValueDigest: kernel.CanonicalDigest(value), OriginalRank: 1,
			RetrievalEvidence: []observability.RefineLaneEvidence{{Provider: "opensearch", Lane: "lexical", LocalRank: 1}},
		}},
		Outcome: "COMPLETED", ModelOutput: &observability.RefineModelOutput{
			Provider: "llm-native", Model: "gpt-5.6-luna", PromptRevision: "listwise-v1", DurationMillis: 25,
			Groups: []observability.RefineRankGroup{{Rank: 1, Refs: []knowledge.KnowledgeRef{ref}}}, Unjudged: []knowledge.KnowledgeRef{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, feedback := range []observability.FeedbackEvent{
		{OccurredAt: "2026-08-31T00:00:00.3Z", Identity: identity, Trace: trace, Workspace: "agent", Outcome: "answered",
			RefineEvidenceID: refineID, LabelSource: "agent", Answer: "Inspect the payment gateway timeout.", SelectedRefs: []knowledge.KnowledgeRef{ref}},
		{OccurredAt: "2026-08-31T00:00:00.4Z", Identity: observability.IdentityContext{Principal: "user:kai"}, Trace: trace,
			Workspace: "agent", Outcome: "accepted", RefineEvidenceID: refineID, LabelSource: "user"},
	} {
		if err := store.RecordFeedback(feedback); err != nil {
			t.Fatal(err)
		}
	}
	view, err := store.Trace("trace-rerank")
	if err != nil || len(view.Entries) != 4 || view.Entries[1].Kind != "refine" {
		t.Fatalf("trace = %#v err=%v", view, err)
	}
	samples, err := store.RerankTrainingSamples(observability.RefineQuery{EvidenceID: refineID})
	if err != nil || len(samples) != 1 || !samples[0].TrainingEligible || samples[0].LabelStrength != "accepted-answer" {
		t.Fatalf("samples = %#v err=%v", samples, err)
	}
	if got := samples[0].Refine.Candidates[0].Value.(map[string]any)["body"]; got != value["body"] {
		t.Fatalf("projected candidate value lost: %#v", samples[0])
	}
	failedID, err := store.RecordRefineReceipt(observability.RefineEvent{
		AccessEvidenceID: accessID, OccurredAt: "2026-08-31T00:00:00.5Z", Identity: identity, Trace: trace,
		Action: "knowledge.rerank", Workspace: "agent",
		SearchView:      observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/runbooks": "c1"}},
		Spec:            observability.RefineSpec{SpecRef: "urn:rank:refund", Revision: 1, Operator: "SEMANTIC_RERANK", Criterion: "refund timeout relevance"},
		CandidateDigest: "digest-2", ProjectedBytes: 128,
		Candidates: []observability.RefineCandidate{{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}, Value: value,
			ValueDigest: kernel.CanonicalDigest(value), OriginalRank: 1,
		}},
		Outcome: "ERROR", Error: map[string]any{"code": "TEMPORARY_UNAVAILABLE", "message": "timeout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := store.Refine(observability.RefineQuery{TraceID: trace.TraceID, Limit: 1})
	if err != nil || len(latest) != 1 || latest[0].EvidenceID != failedID {
		t.Fatalf("latest refine limit = %#v err=%v", latest, err)
	}
	completed, err := store.Refine(observability.RefineQuery{Provider: "llm-native", Model: "gpt-5.6-luna", Outcome: "COMPLETED"})
	if err != nil || len(completed) != 1 || completed[0].EvidenceID != refineID {
		t.Fatalf("filtered refine = %#v err=%v", completed, err)
	}
}

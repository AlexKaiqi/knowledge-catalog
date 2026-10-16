package observability_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/observability"
)

func composedRetrievalEvent() observability.RetrievalEvent {
	request := map[string]any{"query": "gmv"}
	candidates := []observability.RetrievalCandidate{{
		ObjectRef: objectmodel.PinnedObjectRef{ObjectRef: objectmodel.ObjectRef{Object: "metric/gmv"}},
		Sources: []objectmodel.SourceBasis{
			{Repository: "kr://composed/metrics", Commit: "metrics-v1", Aspects: []string{"definition", "caliber"}},
			{Repository: "kr://composed/quality", Commit: "quality-v2", Aspects: []string{"quality"}},
		},
		Rank: 1, ValueDigest: kernel.CanonicalDigest(map[string]any{"definition": "gmv", "caliber": "paid", "quality": "current"}),
		Evidence: []observability.RetrievalLaneEvidence{{Provider: "composed-index", Lane: "lexical", Guarantee: "exact", LocalRank: 1}},
	}}
	return observability.RetrievalEvent{
		SchemaVersion: 1, EvidenceID: "rt_composed_fixture", AccessEvidenceID: "ev_composed_fixture",
		OccurredAt: "2026-09-17T00:00:00.2Z", Identity: observability.IdentityContext{Principal: "agent:consumer"},
		Trace:  observability.TraceContext{TraceID: "trace-composed", SpanID: "span-composed"},
		Action: "file.read", Dataset: "analysis", Operator: observability.RetrievalOperatorSearch,
		LogicalRequest: request, RequestDigest: kernel.CanonicalDigest(request),
		SearchView: observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{
			"kr://composed/metrics": "metrics-v1", "kr://composed/quality": "quality-v2",
		}},
		Completeness: "complete", Candidates: candidates, CandidateDigest: kernel.CanonicalDigest(candidates),
		Execution: observability.RetrievalExecution{Candidates: 1, Hydrated: 1}, Outcome: "COMPLETED",
	}
}

func TestComposedRetrievalSurvivesDurableQueryAndTrainingRebuild(t *testing.T) {
	store := datedFileStore(t)
	event := composedRetrievalEvent()
	targets := []observability.ObjectAccess{}
	for _, source := range event.Candidates[0].Sources {
		targets = append(targets, observability.ObjectAccess{ObjectRef: objectmodel.PinnedObjectRef{
			ObjectRef: objectmodel.ObjectRef{Repository: source.Repository, Object: event.Candidates[0].ObjectRef.Object}, Commit: source.Commit,
		}})
	}
	accessID, err := store.RecordAccessReceipt(observability.AccessEvent{
		OccurredAt: "2026-09-17T00:00:00.1Z", Identity: event.Identity, Trace: event.Trace,
		Action: event.Action, Dataset: event.Dataset, Decision: "ALLOW", Result: "RESOLVED", Object: targets,
	})
	if err != nil {
		t.Fatal(err)
	}
	event.EvidenceID, event.AccessEvidenceID = "", accessID
	id, err := store.RecordRetrievalReceipt(event)
	if err != nil || id == "" || store.LastAppendBytes() <= 0 {
		t.Fatalf("composed candidate did not receive a durable receipt: id=%s error=%v", id, err)
	}
	// Reopen the adapter so no in-memory event can satisfy either readback.
	reopened := observability.NewFileStore(store.Home)
	reopened.Now = store.Now
	query := observability.RetrievalQuery{EvidenceID: id, Provider: "composed-index", Operator: observability.RetrievalOperatorSearch}
	events, err := reopened.Retrieval(query)
	if err != nil || len(events) != 1 || events[0].AccessEvidenceID != accessID || !reflect.DeepEqual(events[0].Candidates, event.Candidates) || events[0].CandidateDigest != event.CandidateDigest || !reflect.DeepEqual(events[0].SearchView, event.SearchView) {
		t.Fatalf("durable query lost the single ranked source vector: events=%#v error=%v", events, err)
	}
	if err := events[0].Validate(); err != nil {
		t.Fatalf("readback cannot reproduce valid composed evidence: %v", err)
	}
	samples, err := reopened.RetrievalTrainingSamples(query)
	if err != nil || len(samples) != 1 || !reflect.DeepEqual(samples[0].Retrieval.Candidates, event.Candidates) || samples[0].TrainingEligible || samples[0].LabelStrength != "none" {
		t.Fatalf("training rebuild lost the source vector or invented feedback: samples=%#v error=%v", samples, err)
	}
	if len(samples[0].Retrieval.Candidates) != 1 || samples[0].Retrieval.Candidates[0].Rank != 1 || len(samples[0].Retrieval.Candidates[0].Sources) != 2 {
		t.Fatalf("one composed hit became several ranked candidates: %#v", samples)
	}
}

func TestComposedRetrievalRejectsInvalidSourceBasis(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*observability.RetrievalEvent)
	}{
		{"missing Dataset", func(e *observability.RetrievalEvent) { e.Dataset = "" }},
		{"blank Dataset", func(e *observability.RetrievalEvent) { e.Dataset = " " }},
		{"missing object", func(e *observability.RetrievalEvent) { e.Candidates[0].ObjectRef.Object = "" }},
		{"single repository", func(e *observability.RetrievalEvent) { e.Candidates[0].ObjectRef.Repository = "kr://composed/metrics" }},
		{"single commit", func(e *observability.RetrievalEvent) { e.Candidates[0].ObjectRef.Commit = "metrics-v1" }},
		{"one source", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources = e.Candidates[0].Sources[:1] }},
		{"missing source repository", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Repository = "" }},
		{"missing source commit", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Commit = "" }},
		{"unknown source", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Repository = "kr://composed/outside" }},
		{"wrong source version", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Commit = "metrics-v2" }},
		{"duplicate repository", func(e *observability.RetrievalEvent) {
			e.Candidates[0].Sources[1].Repository = e.Candidates[0].Sources[0].Repository
		}},
		{"missing Aspects", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Aspects = nil }},
		{"empty Aspect", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Aspects[0] = "" }},
		{"blank Aspect", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Aspects[0] = " " }},
		{"duplicate Aspect", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[0].Aspects[1] = "definition" }},
		{"conflicting Aspect", func(e *observability.RetrievalEvent) { e.Candidates[0].Sources[1].Aspects[0] = "definition" }},
		{"missing SearchView", func(e *observability.RetrievalEvent) { e.SearchView.Snapshots = nil }},
		{"duplicate composed candidate", func(e *observability.RetrievalEvent) {
			duplicate := e.Candidates[0]
			duplicate.Rank = 2
			duplicate.Sources = []objectmodel.SourceBasis{duplicate.Sources[1], duplicate.Sources[0]}
			e.Candidates = append(e.Candidates, duplicate)
		}},
		{"wrong rank", func(e *observability.RetrievalEvent) { e.Candidates[0].Rank = 2 }},
		{"missing value digest", func(e *observability.RetrievalEvent) { e.Candidates[0].ValueDigest = "" }},
		{"missing lane provider", func(e *observability.RetrievalEvent) { e.Candidates[0].Evidence[0].Provider = "" }},
		{"missing lane", func(e *observability.RetrievalEvent) { e.Candidates[0].Evidence[0].Lane = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := composedRetrievalEvent()
			test.mutate(&event)
			event.CandidateDigest = kernel.CanonicalDigest(event.Candidates)
			if err := event.Validate(); err == nil {
				t.Fatalf("invalid composed source basis was accepted: %#v", event)
			}
		})
	}
	event := composedRetrievalEvent()
	event.CandidateDigest = "unrelated-candidate-digest"
	if err := event.Validate(); err == nil {
		t.Fatal("source vectors must participate in the canonical candidate digest")
	}
}

func TestRetrievalWithoutSourcesKeepsSinglePinnedCandidateContract(t *testing.T) {
	event := composedRetrievalEvent()
	event.Dataset = ""
	candidate := &event.Candidates[0]
	candidate.Sources = nil
	candidate.ObjectRef.Repository, candidate.ObjectRef.Commit = "kr://composed/metrics", "metrics-v1"
	event.CandidateDigest = kernel.CanonicalDigest(event.Candidates)
	if err := event.Validate(); err != nil {
		t.Fatalf("legacy single pinned candidate changed: %v", err)
	}
	raw, err := json.Marshal(candidate)
	if err != nil || strings.Contains(string(raw), "\"sources\"") {
		t.Fatalf("legacy single-source wire shape changed: %s error=%v", raw, err)
	}
	for _, missing := range []string{"repository", "commit", "both"} {
		t.Run(missing, func(t *testing.T) {
			invalid := event
			invalid.Candidates = append([]observability.RetrievalCandidate(nil), event.Candidates...)
			if missing != "commit" {
				invalid.Candidates[0].ObjectRef.Repository = ""
			}
			if missing != "repository" {
				invalid.Candidates[0].ObjectRef.Commit = ""
			}
			invalid.CandidateDigest = kernel.CanonicalDigest(invalid.Candidates)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("unpinned candidate without Sources was accepted: %#v", invalid)
			}
		})
	}
}

package invocation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	"kc/kernel"
	"kc/observability"
)

func TestComposedSearchRetainsOneDurableCandidateAndSourceVector(t *testing.T) {
	f := newComposedMemberDeliveryFixture(t)
	flags := composedMemberDeliveryFlags(f)
	flags["trace-id"] = "composed-retrieval-evidence"
	var expected retrieval.ObjectHit
	result := f.invoke("file.read", flags, func(cx *Invocation) (any, error) {
		serving, _, err := OpenCompleteServing(cx, "metric/gmv")
		if err != nil {
			return nil, err
		}
		expected = composedMemberDeliveryHit(t, f, serving)
		expected.Evidence = []retrieval.LaneEvidence{{Provider: "fixture", Lane: "lexical", Guarantee: "exact", LocalRank: 1}}
		delivered, err := DeliverSearchHit(cx.Home, cx.Flags, expected)
		if err != nil {
			return nil, err
		}
		return namedConsumptionSearchOutput(serving.Pin(), delivered), nil
	})
	if result.Status != 0 {
		t.Fatalf("typed composed SEARCH failed before durable evidence: %#v", result)
	}
	var returned retrieval.SearchResult
	if err := json.Unmarshal([]byte(result.Stdout), &returned); err != nil {
		t.Fatal(err)
	}
	if returned.RetrievalEvidenceID == "" || len(returned.Hits) != 1 || !reflect.DeepEqual(returned.Hits[0].Object.Value, expected.Object.Value) {
		t.Fatalf("typed response lost the composed hit or durable receipt: %#v", returned)
	}
	// A fresh store handle must recover what the central evidence chain acked.
	store := observability.NewFileStore(f.ws.Dir)
	events, err := store.Retrieval(observability.RetrievalQuery{EvidenceID: returned.RetrievalEvidenceID, Provider: "fixture"})
	if err != nil || len(events) != 1 {
		t.Fatalf("durable composed retrieval was not readable: events=%#v error=%v", events, err)
	}
	event := events[0]
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if event.Dataset != "analysis" || len(event.Candidates) != 1 || !reflect.DeepEqual(event.SearchView.Snapshots, f.pin.Repositories) {
		t.Fatalf("evidence changed logical candidate count or fixed scope: %#v", event)
	}
	candidate := event.Candidates[0]
	if candidate.Rank != 1 || candidate.ObjectRef.Object != "metric/gmv" || candidate.ObjectRef.Repository != "" || candidate.ObjectRef.Commit != "" ||
		!reflect.DeepEqual(candidate.Sources, expected.Object.Sources) || !reflect.DeepEqual(candidate.Observations, expected.Version.Observations) ||
		candidate.ValueDigest != kernel.CanonicalDigest(expected.Object.Value) || event.CandidateDigest != kernel.CanonicalDigest(event.Candidates) {
		t.Fatalf("evidence invented a single pin or lost the source vector: %#v", event)
	}
	access, found, err := store.GetAccess(context.Background(), event.AccessEvidenceID)
	if err != nil || !found || access.Identity != event.Identity || access.RequestID != event.RequestID || access.PinID != f.pin.PinID || len(access.Object) != 2 {
		t.Fatalf("retrieval did not join its original source access receipt: access=%#v found=%v error=%v", access, found, err)
	}
	trace, err := store.Trace(event.Trace.TraceID)
	if err != nil || len(trace.Entries) != 2 || trace.Entries[1].Retrieval == nil || !reflect.DeepEqual(trace.Entries[1].Retrieval.Candidates, event.Candidates) {
		t.Fatalf("trace lost durable composed evidence: %#v error=%v", trace, err)
	}
	samples, err := store.RetrievalTrainingSamples(observability.RetrievalQuery{EvidenceID: event.EvidenceID})
	if err != nil || len(samples) != 1 || !reflect.DeepEqual(samples[0].Retrieval.Candidates, event.Candidates) || samples[0].TrainingEligible {
		t.Fatalf("derived view changed the unlabeled candidate source facts: %#v error=%v", samples, err)
	}
}

func TestComposedRetrievalEventOwnsItsSourceVector(t *testing.T) {
	f := newComposedMemberDeliveryFixture(t)
	result := f.invoke("file.read", composedMemberDeliveryFlags(f), func(cx *Invocation) (any, error) {
		serving, _, err := OpenCompleteServing(cx, "metric/gmv")
		if err != nil {
			return nil, err
		}
		hit := composedMemberDeliveryHit(t, f, serving)
		search := namedConsumptionSearchOutput(serving.Pin(), hit)
		event, err := RetrievalEventFrom("object-search", cx.Flags, search, "ev_fixed", nil)
		if err != nil {
			return nil, err
		}
		if len(event.Candidates) != 1 || len(event.Candidates[0].Sources) != 2 || len(event.Candidates[0].Sources[0].Aspects) == 0 {
			t.Fatalf("event did not retain the composed source vector: %#v", event)
		}
		search.Hits[0].Object.Sources[0].Aspects[0] = "changed-after-event"
		search.Hits[0].Object.Sources[1].Commit = "changed-after-event"
		if event.Candidates[0].Sources[0].Aspects[0] != "definition" || event.Candidates[0].Sources[1].Commit != f.pin.Repositories[f.repos[1].ID()] ||
			event.CandidateDigest != kernel.CanonicalDigest(event.Candidates) {
			t.Fatalf("event retained mutable source vector storage: %#v", event)
		}
		return namedConsumptionSearchOutput(serving.Pin()), nil
	})
	if result.Status != 0 {
		t.Fatalf("source vector ownership probe failed: %#v", result)
	}
}

func TestComposedSearchCannotAcknowledgeInvalidSourceEvidence(t *testing.T) {
	for _, scenario := range []string{"unknown source", "wrong commit", "single pin"} {
		t.Run(scenario, func(t *testing.T) {
			f := newComposedMemberDeliveryFixture(t)
			called := false
			result := f.invoke("file.read", composedMemberDeliveryFlags(f), func(cx *Invocation) (any, error) {
				called = true
				serving, _, err := OpenCompleteServing(cx, "metric/gmv")
				if err != nil {
					return nil, err
				}
				hit := composedMemberDeliveryHit(t, f, serving)
				switch scenario {
				case "unknown source":
					hit.Object.Sources[1].Repository = "kr://consumption/outside"
				case "wrong commit":
					hit.Object.Sources[1].Commit = "not-the-accepted-commit"
				case "single pin":
					hit.Object.Repository, hit.Object.Commit = f.repos[0].ID(), f.pin.Repositories[f.repos[0].ID()]
				}
				// Fault injection after consumption models a corrupt executor result.
				// The production evidence chain must reject it before success ack.
				return namedConsumptionSearchOutput(serving.Pin(), hit), nil
			})
			if result.Status == 0 || !called {
				t.Fatalf("invalid source evidence received a success ack: called=%v result=%#v", called, result)
			}
			events, err := observability.NewFileStore(f.ws.Dir).Retrieval(observability.RetrievalQuery{})
			if err != nil || len(events) != 0 {
				t.Fatalf("invalid source vector entered durable retrieval evidence: %#v error=%v", events, err)
			}
		})
	}
}

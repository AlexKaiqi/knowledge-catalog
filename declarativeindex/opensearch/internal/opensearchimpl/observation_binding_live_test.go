package opensearchimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

func TestLiveOpenSearchObservationContinuationKeepsOriginalValuesAndExpiryAfterReplacement(t *testing.T) {
	e, transport := publicationRecoveryEngine(t)
	docs := publicationRecoveryDocs()
	second := docs[0]
	second.ObjectID = "article/two"
	docs = append(docs, second)
	third := docs[0]
	third.ObjectID = "article/three"
	docs = append(docs, third)
	meta := ports.Meta{Basis: "fixed", Cause: "observation", Revision: "observation-r1", ObservationDigest: "observed-values-r1"}
	if err := e.Rebuild(docs, meta); err != nil {
		t.Fatal(err)
	}
	// Reproduce the durable count shape written by the earlier owned codec.
	accepted, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	oldResource := &rebuildSession{engine: e, generation: accepted.Generation, physicalIndex: accepted.ActiveIndex}
	if err := oldResource.changeResourceRecord(func(record *controlDoc) error { record.ObjectCount = 0; return nil }); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bound, err := e.BindObservation(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	request := ports.RetrieveRequest{Search: retrieval.SearchOf(retrieval.SearchMATCH("runbook")), Spec: retrieval.AccessSpec{Repository: "logical-observation", Commit: "fixed", Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}}}}
	request.Search.Limit = 1
	first, err := bound.(ports.ContextRetriever).RetrieveContext(ctx, request)
	if err != nil || len(first.Candidates) != 1 || first.Continuation == "" {
		t.Fatalf("first observation page: %#v %v", first, err)
	}
	original, err := decodePITContinuation(first.Continuation)
	if err != nil || original.Lease == "" {
		t.Fatalf("no original protection: %#v %v", original, err)
	}
	for _, change := range []func(*pitContinuation){
		func(cursor *pitContinuation) { cursor.Lease = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		func(cursor *pitContinuation) { cursor.Until += 1000 },
		func(cursor *pitContinuation) { cursor.Namespace = "foreign" },
	} {
		forged := original
		change(&forged)
		if _, err := e.BindObservation(ctx, encodePITContinuation(forged)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("forged observation protection admitted: %#v %v", forged, err)
		}
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	meta.Revision, meta.ObservationDigest = "observation-r2", "observed-values-r2"
	if err := e.Rebuild(nil, meta); err != nil {
		t.Fatal(err)
	}
	current, err := e.BindObservation(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := current.Count(); err != nil || n != 0 {
		t.Fatalf("new query did not use new observation: %d %v", n, err)
	}
	_ = current.Close()
	peer := reopenResourceEngine(t, e, transport)
	resumed, err := peer.BindObservation(ctx, first.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	got, err := resumed.LoadMeta()
	if err != nil || got.Revision != "observation-r1" || got.Generation != original.Generation {
		t.Fatalf("resume selected new observation: %#v %v", got, err)
	}
	request.Continuation = first.Continuation
	page, err := resumed.(ports.ContextRetriever).RetrieveContext(ctx, request)
	if err != nil || len(page.Candidates) != 1 || page.Candidates[0].ObjectID == first.Candidates[0].ObjectID || page.Continuation == "" {
		t.Fatalf("resumed wrong page: %#v %v", page, err)
	}
	next, err := decodePITContinuation(page.Continuation)
	if err != nil || next.Lease != original.Lease || next.Until != original.Until {
		t.Fatalf("resume renewed original protection: %#v %v", next, err)
	}
	// Simulate authoritative expiry without a two-minute wall-clock test. A
	// query admitted before expiry must keep its independent in-flight lease.
	root, version, err := peer.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	root.ObservationDeadlines[original.Generation] = time.Now().Add(-time.Second).UnixMilli()
	if _, err := peer.putControl(root, version); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.BindObservation(ctx, page.Continuation); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("expired observation admitted: %v", err)
	}
	if n, err := resumed.Count(); err != nil || n != 3 {
		t.Fatalf("expiry removed admitted request protection: %d %v", n, err)
	}
	if _, err := resumed.(ports.StreamingProjectionMaintainer).BeginRebuild(meta); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("fixed observation view is mutable: %v", err)
	}
}

func TestObservationDeadlineRejectsMissingProofAndMetadataChanges(t *testing.T) {
	now := time.Now().UnixMilli()
	record := controlDoc{Generation: "original", State: ports.ProjectionStateReady, Cause: "observation", Basis: "fixed"}
	for _, root := range []controlDoc{
		{Generation: "next", State: ports.ProjectionStateReady, Cause: "observation"},
		{Generation: "next", State: ports.ProjectionStateReady, Cause: "observation", ObservationDeadlines: map[string]int64{"original": now}},
		{Generation: "original", State: ports.ProjectionStateReady, Cause: "observation", Basis: "changed"},
	} {
		if _, err := observationDeadline(root, record, now); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("unknown observation accepted: %#v %v", root, err)
		}
	}
}

func TestLiveOpenSearchObservationBindingUsesConfirmedIncrementalMetadata(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	docs := publicationRecoveryDocs()
	meta := ports.Meta{Basis: "fixed", Cause: "observation", Revision: "r1", ObservationDigest: "values-r1"}
	if err := e.Rebuild(docs, meta); err != nil {
		t.Fatal(err)
	}
	addition := docs[0]
	addition.ObjectID = "article/added"
	meta.Revision, meta.ObservationDigest = "r2", "values-r2"
	if err := e.Apply([]ports.CompiledDoc{addition}, nil, meta); err != nil {
		t.Fatal(err)
	}
	bound, err := e.BindObservation(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	got, err := bound.LoadMeta()
	if err != nil || got.Revision != "r2" || got.ObservationDigest != "values-r2" {
		t.Fatalf("incremental resource metadata is stale: %#v %v", got, err)
	}
	if count, err := bound.Count(); err != nil || count != 2 {
		t.Fatalf("incremental resource count is stale: %d %v", count, err)
	}
}

func TestObservationBindingRejectsForeignContinuationBeforeBackendRead(t *testing.T) {
	var reads atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reads.Add(1)
		http.Error(w, "must not be called", http.StatusServiceUnavailable)
	}))
	defer backend.Close()
	engine, err := Open(Config{URL: backend.URL, Namespace: "owned-observations"})(t.TempDir(), "scope")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	token := encodePITContinuation(pitContinuation{Generation: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Namespace: namespaceDigest("foreign-observations")})
	if _, err := engine.(ports.ObservationBinder).BindObservation(context.Background(), token); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || reads.Load() != 0 {
		t.Fatalf("foreign cursor reached backend: reads=%d err=%v", reads.Load(), err)
	}
}

package opensearchimpl

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func TestLiveOpenSearchPublicationContinuationRetainsOriginalGenerationAcrossRepairAndReopen(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	docs := publicationRecoveryDocs()
	for _, id := range []string{"article/two", "article/three"} {
		doc := docs[0]
		doc.ObjectID = objectmodel.ObjectID(id)
		docs = append(docs, doc)
	}
	if err := e.Rebuild(docs, ports.Meta{Basis: "fixed"}); err != nil {
		t.Fatal(err)
	}
	meta, err := e.LoadMeta()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareResource(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}
	ticket := prepared.(*preparedResource)
	ctx := context.Background()
	evidence := []byte("fixed-publication-with-continuation")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	bound, err := e.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	request := ports.RetrieveRequest{Search: retrieval.SearchOf(retrieval.SearchMATCH("runbook")), Spec: retrieval.AccessSpec{Repository: "logical-query-scope", Commit: "fixed", Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}}}}
	request.Search.Limit = 1
	first, err := bound.(ports.ContextRetriever).RetrieveContext(ctx, request)
	if err != nil || len(first.Candidates) != 1 || first.Continuation == "" {
		t.Fatalf("first protected page: %#v %v", first, err)
	}
	original, err := decodePITContinuation(first.Continuation)
	if err != nil || original.Lease == "" || original.Until == 0 {
		t.Fatalf("page has no durable continuation: %#v %v", original, err)
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	t.Run("authority-cutoff-fences-query-admission", func(t *testing.T) {
		cutoff := time.Now().Add(time.Second)
		admitted, err := e.BindPublicationContinuationUntil(ctx, evidence, first.Continuation, cutoff)
		if err != nil {
			t.Fatal(err)
		}
		defer admitted.Close()
		paused := &pausedPublicationQueryAdmission{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + ticket.session.resourceID()}
		e.http.Transport = paused
		_, err = e.BindPublicationContinuationUntil(ctx, evidence, first.Continuation, cutoff)
		e.http.Transport = resources
		if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !paused.hit {
			t.Fatalf("query CAS escaped original cutoff: hit=%v %v", paused.hit, err)
		}
		if count, err := admitted.Count(); err != nil || count != 3 {
			t.Fatalf("cutoff ended already admitted request: %d %v", count, err)
		}
		if _, err := e.BindPublicationContinuationUntil(ctx, evidence, first.Continuation, cutoff); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("expired observation admitted another query: %v", err)
		}
	})
	// Only the accepted map is lost. Repair builds an empty replacement corpus;
	// neither active pointer is a legal substitute for the original continuation.
	if status, _, err := e.do(http.MethodDelete, "/"+e.controlIndexPath()+"/_doc/"+url.PathEscape(e.publicationBindingID(evidence)), nil); err != nil || status >= 300 {
		t.Fatalf("remove binding: %d %v", status, err)
	}
	if err := e.RepairPublication(ctx, evidence, ports.Meta{}, func(context.Context) (ports.Meta, error) {
		if err := e.Rebuild(nil, ports.Meta{Basis: "fixed"}); err != nil {
			return ports.Meta{}, err
		}
		return e.LoadMeta()
	}, func(apply func() error) error { return apply() }); err != nil {
		t.Fatal(err)
	}
	current, err := e.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := current.Count(); err != nil || count != 0 {
		t.Fatalf("replacement corpus not actually empty: %d %v", count, err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ticket.session.changeResource(func(resource *generationResource) error {
		resource.Claims = slices.DeleteFunc(resource.Claims, func(claim resourceClaim) bool { return claim.ID == ticket.id })
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if status, _, err := e.do(http.MethodDelete, "/"+e.controlIndexPath()+"/_doc/"+url.PathEscape(e.publicationBindingID(evidence)), nil); err != nil || status >= 300 {
		t.Fatalf("remove replacement binding: %d %v", status, err)
	}
	peer := reopenResourceEngine(t, e, resources)
	seen := map[string]bool{string(first.Candidates[0].ObjectID): true}
	token := first.Continuation
	for token != "" {
		resumed, err := peer.BindPublicationContinuation(ctx, evidence, token)
		if err != nil {
			t.Fatal(err)
		}
		if count, err := resumed.Count(); err != nil || count != 3 {
			t.Fatalf("fixed continuation lost original corpus count: %d %v", count, err)
		}
		request.Continuation = token
		page, err := resumed.(ports.ContextRetriever).RetrieveContext(ctx, request)
		closeErr := resumed.Close()
		if err != nil || closeErr != nil || len(page.Candidates) != 1 {
			t.Fatalf("continuation followed replacement: %#v %v %v", page, err, closeErr)
		}
		for _, candidate := range page.Candidates {
			if seen[string(candidate.ObjectID)] {
				t.Fatal("continuation repeated a candidate")
			}
			seen[string(candidate.ObjectID)] = true
		}
		token = page.Continuation
		if token != "" {
			after, err := decodePITContinuation(token)
			if err != nil || after.Lease != original.Lease || after.Until != original.Until {
				t.Fatalf("continuation renewed original deadline: %#v %v", after, err)
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("continuation lost fixed corpus: %#v", seen)
	}
	// A public checksum does not grant authority to extend a durable lease.
	extended := original
	extended.Until += int64(time.Hour / time.Millisecond)
	if _, err := peer.BindPublicationContinuation(ctx, evidence, encodePITContinuation(extended)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("caller extended continuation deadline: %v", err)
	}
	for _, change := range []func(*pitContinuation){
		func(state *pitContinuation) { state.Query = "another-query" },
		func(state *pitContinuation) { state.Repository = "another-repository" },
		func(state *pitContinuation) { state.Namespace = kernel.CanonicalDigest("another-instance") },
	} {
		altered := original
		change(&altered)
		if _, err := peer.BindPublicationContinuation(ctx, evidence, encodePITContinuation(altered)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("continuation scope was caller-replaceable: %v", err)
		}
	}
	inflight, err := peer.BindPublicationContinuation(ctx, evidence, first.Continuation)
	if err != nil {
		t.Fatal(err)
	}
	resource, version, err := peer.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	for i := range resource.Resource.Claims {
		if resource.Resource.Claims[i].ID == original.Lease {
			resource.Resource.Claims[i].Until = time.Now().Add(-time.Second).UnixMilli()
		}
	}
	if _, err := peer.putControlRecordContext(ctx, ticket.session.resourceID(), resource, version); err != nil {
		t.Fatal(err)
	}
	report, err := peer.RecoverPublications(ctx, func(context.Context, []byte) (bool, error) {
		t.Fatal("continuation expiry used publication authority")
		return false, nil
	})
	if err != nil || len(report.Released) != 1 {
		t.Fatalf("continuation expiry did not recover: %#v %v", report, err)
	}
	if _, err := peer.BindPublicationContinuation(ctx, evidence, first.Continuation); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("expired continuation started a new query: %v", err)
	}
	request.Continuation = first.Continuation
	page, err := inflight.(ports.ContextRetriever).RetrieveContext(ctx, request)
	if err != nil || len(page.Candidates) != 1 {
		t.Fatalf("continuation expiry ended already admitted in-flight query: %#v %v", page, err)
	}
	if err := inflight.Close(); err != nil {
		t.Fatal(err)
	}

}

// The actual resource CAS waits past the authority cutoff; cancellation must
// abort admission, while an earlier independent query remains usable.
type pausedPublicationQueryAdmission struct {
	next http.RoundTripper
	path string
	hit  bool
}

func (p *pausedPublicationQueryAdmission) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPut && request.URL.Path == p.path {
		p.hit = true
		<-request.Context().Done()
		return nil, request.Context().Err()
	}
	return p.next.RoundTrip(request)
}

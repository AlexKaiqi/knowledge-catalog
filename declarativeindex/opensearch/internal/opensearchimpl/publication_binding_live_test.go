package opensearchimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func TestLiveOpenSearchPublicationBindingSurvivesReplacementAndReopen(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	expected, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("immutable-publication-evidence")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	replacement := expected
	replacement.Basis = "another-physical-basis"
	docs := append(publicationRecoveryDocs(), ports.CompiledDoc{ObjectID: "article/two", Text: "different"})
	if err := e.Rebuild(docs, replacement); err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	bound, err := peer.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	actual, err := bound.LoadMeta()
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("bound query followed mutable pointer: %#v expected=%#v err=%v", actual, expected, err)
	}
	if count, err := bound.Count(); err != nil || count != 1 {
		t.Fatalf("bound count lost original generation: %d %v", count, err)
	}
	if count, err := peer.Count(); err != nil || count != 2 {
		t.Fatalf("test did not actually replace current generation: %d %v", count, err)
	}
	current, _, err := peer.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	// Even loss of the mutable root cannot redirect the accepted generation.
	if status, _, err := peer.do(http.MethodDelete, "/"+peer.controlIndexPath()+"/_doc/"+url.PathEscape(peer.controlID), nil); err != nil || status >= 300 {
		t.Fatalf("remove test pointer: %d %v", status, err)
	}
	again, err := peer.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if count, err := again.Count(); err != nil || count != 1 {
		t.Fatalf("accepted query required mutable pointer: %d %v", count, err)
	}
	for _, mutate := range []func() error{
		func() error { return bound.Rebuild(nil, ports.Meta{}) },
		func() error { return bound.Apply(nil, nil, ports.Meta{}) },
		func() error {
			_, err := bound.(ports.StreamingProjectionMaintainer).BeginRebuild(ports.Meta{})
			return err
		},
		func() error { _, err := bound.(ports.ResourcePreparer).PrepareResource(ctx, expected); return err },
		func() error { _, err := bound.(ports.ResourceMaintainer).RecoverResources(ctx); return err },
	} {
		if err := mutate(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("read-only publication view mutated state: %v", err)
		}
	}
	requireGenerationStatus(t, peer, current.ActiveIndex, http.StatusOK)
	requireGenerationStatus(t, peer, ticket.session.physicalIndex, http.StatusOK)
}

func TestLiveOpenSearchPublicationBindingNeverFallsBackOnMissingProtection(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("binding-must-exist")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if status, _, err := e.do(http.MethodDelete, "/"+e.controlIndexPath()+"/_doc/"+url.PathEscape(e.publicationBindingID(evidence)), nil); err != nil || status >= 300 {
		t.Fatalf("remove test binding: %d %v", status, err)
	}
	if _, err := e.BindPublication(ctx, evidence); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing binding fell back to READY pointer: %v", err)
	}
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	expirePublicationClaim(t, ticket)
	if _, err := resourceAuthorityForTest(t, e).RecoverPublications(ctx, func(context.Context, []byte) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := e.BindPublication(ctx, evidence); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("released reference remained a query capability: %v", err)
	}
	requireGenerationStatus(t, e, ticket.session.physicalIndex, http.StatusOK)
}

func TestLiveOpenSearchUpdatedResourceCapturesMetadataBeforePublication(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(publicationRecoveryDocs(), nil, ports.Meta{Basis: "after"}); err != nil {
		t.Fatal(err)
	}
	meta, err := e.LoadMeta()
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := e.PrepareResource(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := ticket.Admit(context.Background(), []byte("updated-resource")); err != nil {
		t.Fatal(err)
	}
	bound, err := e.BindPublication(context.Background(), []byte("updated-resource"))
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if actual, err := bound.LoadMeta(); err != nil || !reflect.DeepEqual(actual, meta) {
		t.Fatalf("updated resource retained stale pre-update metadata: %#v %v", actual, err)
	}
}

type losePublicationBindingResponse struct {
	next http.RoundTripper
	path string
	lost atomic.Bool
}

func (l *losePublicationBindingResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := l.next.RoundTrip(request)
	if err == nil && request.Method == http.MethodPut && request.URL.Path == l.path && response.StatusCode < 300 && l.lost.CompareAndSwap(false, true) {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, fmt.Errorf("injected response loss after publication binding accepted")
	}
	return response, err
}

func TestLiveOpenSearchLostBindingConfirmationKeepsExactAdmittedResource(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	expected, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("lost-binding-confirmation")
	fault := &losePublicationBindingResponse{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + e.publicationBindingID(evidence)}
	e.http.Transport = fault
	if err := ticket.Admit(ctx, evidence); err == nil || !fault.lost.Load() {
		t.Fatalf("binding acceptance was not made uncertain: %v", err)
	}
	peer := reopenResourceEngine(t, e, resources)
	bound, err := peer.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if actual, err := bound.LoadMeta(); err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("lost binding response redirected resource: %#v %v", actual, err)
	}
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatalf("same immutable binding was not retryable: %v", err)
	}
	if err := ticket.Cancel(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("unknown admitted binding became cancellable: %v", err)
	}
}

func TestLiveOpenSearchPublicationQueryPreservesEmbeddingImplementationIdentity(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	e.embedder, e.vectorDimension, e.vectorModel = sourceBasisEmbedder{}, 1, "identity-model-v1"
	e.embeddingIdentity = "driver-endpoint-model-revision-v1"
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("embedding-implementation-publication")
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
	defer bound.Close()
	provider, ok := bound.(ports.ProviderIdentity)
	if !ok || provider.ProviderRevision() != e.ProviderRevision() || provider.PhysicalDigest() != e.PhysicalDigest() {
		t.Fatalf("fixed publication view changed its prepared provider identity: provider=%v expected=%s", provider, e.PhysicalDigest())
	}
	// The implementation identity must contribute independently of model and dimensions.
	e.embeddingIdentity = "another-driver-revision"
	if provider.PhysicalDigest() == e.PhysicalDigest() {
		t.Fatal("model implementation change was lost from physical compatibility")
	}
}

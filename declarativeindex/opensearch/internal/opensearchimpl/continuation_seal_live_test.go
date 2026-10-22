package opensearchimpl

import (
	"context"
	"encoding/base64"
	"io"
	ports "kc/discovery/projection"
	"net/http"
	"strings"
	"testing"
	"time"

	"kc/kernel"
)

func TestLiveOpenSearchContinuationSealingSurvivesReopenAndRejectsMissingOrForeignMaterial(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, _ = preparePublicationResource(t, e)
	ctx := context.Background()
	id := e.controlID + "-cursor-key"
	if _, err := e.LoadContinuationSealer(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing signing material was invented: %v", err)
	}
	if _, version, err := e.loadControlRecord(ctx, id); err != nil || version != nil {
		t.Fatalf("verification created signing material: %v", err)
	}
	sealer, err := e.PrepareContinuationSealer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"fixed":"opaque-provider-position"}`)
	token := sealer.SealContinuation(body)
	peer := reopenResourceEngine(t, e, resources)
	reopened, err := peer.LoadContinuationSealer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := reopened.OpenContinuation(token); err != nil || string(decoded) != string(body) {
		t.Fatalf("cold verification lost original cursor: %v", err)
	}
	signed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	signed[0] ^= 1
	if _, err := reopened.OpenContinuation(base64.RawURLEncoding.EncodeToString(signed)); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("tampered cursor authenticated: %v", err)
	}
	foreign := peer.resourceScope("another-physical-scope")
	other, err := foreign.PrepareContinuationSealer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.OpenContinuation(token); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("another scope authenticated cursor: %v", err)
	}
	record, version, err := e.loadControlRecord(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	record.CursorKey = []byte("corrupt")
	if _, err := e.putControlRecordContext(ctx, id, record, version); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.PrepareContinuationSealer(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("preparation replaced corrupt material: %v", err)
	}
	if _, err := peer.LoadContinuationSealer(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("verification replaced corrupt material: %v", err)
	}
	after, _, err := peer.loadControlRecord(ctx, id)
	if err != nil || string(after.CursorKey) != "corrupt" {
		t.Fatalf("unknown signing material was overwritten: %v", err)
	}
}

func TestLiveOpenSearchContinuationKeyCreateRecoversLostAcknowledgement(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, _ = preparePublicationResource(t, e)
	lost := &losePublicationBindingResponse{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + e.controlID + "-cursor-key"}
	e.http.Transport = lost
	sealer, err := e.PrepareContinuationSealer(context.Background())
	if err != nil || !lost.lost.Load() {
		t.Fatalf("durable key creation was not recovered: %v", err)
	}
	token := sealer.SealContinuation([]byte("fixed-cursor"))
	peer := reopenResourceEngine(t, e, resources)
	verifier, err := peer.LoadContinuationSealer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if body, err := verifier.OpenContinuation(token); err != nil || string(body) != "fixed-cursor" {
		t.Fatalf("lost acknowledgement used an undurable key: %v", err)
	}
	peer.http.Transport = unavailableCursorKeyRead{next: resources, path: "/" + peer.controlIndexPath() + "/_doc/" + peer.controlID + "-cursor-key"}
	if _, err := peer.LoadContinuationSealer(context.Background()); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("unavailable key verification lost original error: %v", err)
	}
	if _, err := peer.PrepareContinuationSealer(context.Background()); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("unavailable key preparation invented a new secret: %v", err)
	}
	peer.http.Transport = resources
	// Loading unavailable metadata must preserve the error, rather than prepare
	// fresh material that would make an old cursor impossible to authenticate.
	if status, _, err := peer.do(http.MethodDelete, "/"+peer.controlIndexPath()+"/_doc/"+peer.controlID+"-cursor-key", nil); err != nil || status >= 300 {
		t.Fatalf("remove key fixture: %d %v", status, err)
	}
	if _, err := peer.LoadContinuationSealer(context.Background()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing key fell back to a new secret: %v", err)
	}
}

type initialCursorReadBarrier struct {
	next    http.RoundTripper
	path    string
	reached chan struct{}
	release <-chan struct{}
}

func (b *initialCursorReadBarrier) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := b.next.RoundTrip(request)
	if err == nil && request.Method == http.MethodGet && request.URL.Path == b.path && response.StatusCode == http.StatusNotFound {
		b.reached <- struct{}{}
		select {
		case <-b.release:
		case <-request.Context().Done():
		}
	}
	return response, err
}

func TestLiveOpenSearchConcurrentCursorKeyPreparationAdoptsOneDurableWinner(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, _ = preparePublicationResource(t, e)
	peer := reopenResourceEngine(t, e, resources)
	release := make(chan struct{})
	barrier := &initialCursorReadBarrier{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + e.controlID + "-cursor-key", reached: make(chan struct{}, 2), release: release}
	e.http.Transport, peer.http.Transport = barrier, barrier
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	type result struct {
		sealer ports.ContinuationSealer
		err    error
	}
	done := make(chan result, 2)
	for _, engine := range []*openSearchEngine{e, peer} {
		go func() {
			sealer, err := engine.PrepareContinuationSealer(context.Background())
			done <- result{sealer, err}
		}()
	}
	for range 2 {
		select {
		case <-barrier.reached:
		case outcome := <-done:
			t.Fatalf("creator did not observe absent key before race: %v", outcome.err)
		case <-time.After(5 * time.Second):
			t.Fatal("cursor-key creators did not reach actual medium")
		}
	}
	close(release)
	first, second := <-done, <-done
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent key creation failed: %v %v", first.err, second.err)
	}
	actual, err := reopenResourceEngine(t, e, resources).LoadContinuationSealer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, sealer := range []ports.ContinuationSealer{first.sealer, second.sealer} {
		token := sealer.SealContinuation([]byte("one-adopted-key"))
		if body, err := actual.OpenContinuation(token); err != nil || string(body) != "one-adopted-key" {
			t.Fatalf("loser returned an unadopted signing key: %v", err)
		}
	}
}

type unavailableCursorKeyRead struct {
	next http.RoundTripper
	path string
}

func (u unavailableCursorKeyRead) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodGet && request.URL.Path == u.path {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable signing metadata")), Request: request}, nil
	}
	return u.next.RoundTrip(request)
}

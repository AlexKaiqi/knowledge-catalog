package opensearchimpl

import (
	"context"
	"errors"
	"net/http"
	"testing"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func TestLiveOpenSearchAcceptedReferenceReleasePreservesOtherConsumersAndInflightQueries(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	meta, first := preparePublicationResource(t, e)
	ctx := context.Background()
	another, err := e.PrepareResource(ctx, meta)
	if err != nil {
		t.Fatal(err)
	}
	second := another.(*preparedResource)
	for _, entry := range []struct {
		ticket *preparedResource
		proof  string
	}{{first, "expired-observation"}, {second, "still-live-dataset"}} {
		if err := entry.ticket.Admit(ctx, []byte(entry.proof)); err != nil {
			t.Fatal(err)
		}
		if err := entry.ticket.Confirm(ctx); err != nil {
			t.Fatal(err)
		}
	}
	inflight, err := e.BindPublication(ctx, []byte("expired-observation"))
	if err != nil {
		t.Fatal(err)
	}
	defer inflight.Close()
	// Use a fresh instance coordinator: no query engine inventory or source
	// demand may be necessary to find the accepted references.
	manager := resourceAuthorityForTest(t, e).(ports.PublicationReferenceMaintainer)
	calls := 0
	report, err := manager.ReconcilePublicationReferences(ctx, func(_ context.Context, proof []byte) (bool, error) {
		calls++
		return string(proof) == "still-live-dataset", nil
	})
	if err != nil || calls != 2 || len(report.Released) != 1 || len(report.Retained) != 0 {
		t.Fatalf("release lost a consumer: %#v calls=%d %v", report, calls, err)
	}
	if count, err := inflight.Count(); err != nil || count != 1 {
		t.Fatalf("release invalidated admitted query: %d %v", count, err)
	}
	if _, err := e.BindPublication(ctx, []byte("expired-observation")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("released identity admitted new query: %v", err)
	}
	if err := first.Admit(ctx, []byte("expired-observation")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old publisher resurrected released reference: %v", err)
	}
	if err := first.Confirm(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old confirmation resurrected released reference: %v", err)
	}
	live, err := e.BindPublication(ctx, []byte("still-live-dataset"))
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	requireGenerationStatus(t, e, first.session.physicalIndex, http.StatusOK)
	report, err = manager.ReconcilePublicationReferences(ctx, func(_ context.Context, proof []byte) (bool, error) {
		if string(proof) != "still-live-dataset" {
			t.Errorf("released identity was enumerated again: %q", proof)
		}
		return false, nil
	})
	if err != nil || len(report.Released) != 1 {
		t.Fatalf("last publication reference was not released: %#v %v", report, err)
	}
	if count, err := inflight.Count(); err != nil || count != 1 {
		t.Fatalf("last reference release removed in-flight query: %d %v", count, err)
	}
	record, _, err := e.loadControlRecord(ctx, first.session.resourceID())
	if err != nil || len(record.Resource.Claims) != 1 || record.Resource.Claims[0].Phase != claimQuery {
		t.Fatalf("release touched another protection: %#v %v", record.Resource, err)
	}
	if err := inflight.Close(); err != nil {
		t.Fatal(err)
	}
	record, _, err = e.loadControlRecord(ctx, first.session.resourceID())
	if err != nil || len(record.Resource.Claims) != 0 {
		t.Fatalf("last independent query was not released: %#v %v", record.Resource, err)
	}
}

func TestLiveOpenSearchAcceptedReferenceUnknownAndConcurrentChangesRetainProtection(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	if err := ticket.Admit(ctx, []byte("known-reference")); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	unknown := errors.New("consumer authority unavailable")
	report, err := e.ReconcilePublicationReferences(ctx, func(context.Context, []byte) (bool, error) { return false, unknown })
	if !errors.Is(err, unknown) || len(report.Retained) != 1 || len(report.Released) != 0 {
		t.Fatalf("unknown authority discharged protection: %#v %v", report, err)
	}
	report, err = e.ReconcilePublicationReferences(ctx, func(context.Context, []byte) (bool, error) {
		// Authority IO runs outside the provider CAS. A changed claim must not be
		// removed by a decision made for its old immutable identity.
		err := ticket.session.changeResource(func(resource *generationResource) error {
			resource.Claims[0].Evidence = []byte("different-reference")
			return nil
		})
		return false, err
	})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(report.Retained) != 1 || len(report.Released) != 0 {
		t.Fatalf("stale release decision removed changed claim: %#v %v", report, err)
	}
	record, _, err := e.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil || len(record.Resource.Claims) != 1 || string(record.Resource.Claims[0].Evidence) != "different-reference" {
		t.Fatalf("changed claim disappeared: %#v %v", record.Resource, err)
	}
}

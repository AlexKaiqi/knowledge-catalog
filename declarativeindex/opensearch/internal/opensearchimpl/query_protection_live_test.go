package opensearchimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	"kc/kernel"
)

func queryClaims(t *testing.T, ticket *preparedResource) []resourceClaim {
	t.Helper()
	resource, _, err := ticket.session.engine.loadControlRecord(context.Background(), ticket.session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	if resource.Resource.Version != resourceRecordVersion {
		t.Fatalf("query did not acquire current durable resource protocol: %#v", resource.Resource)
	}
	return resource.Resource.Claims
}

func TestLiveOpenSearchQueryProtectionIsDurableBoundedAndIndependentlyReleased(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("query-resource-publication")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	// A previously supported version-2 publication is upgraded by the same CAS
	// that acquires the query, preserving its adopted reference.
	record, version, err := e.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	record.Resource.Version = 2
	record.Resource.QueryLeases = 0
	if _, err := e.putControlRecordContext(ctx, ticket.session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(ctx)
	bound, err := e.BindPublication(request, evidence)
	if err != nil {
		t.Fatal(err)
	}
	claims := queryClaims(t, ticket)
	if len(claims) != 2 || claims[0].ID != ticket.id || claims[0].Phase != claimAccepted || claims[1].Phase != claimQuery || claims[1].Until > time.Now().Add(30*time.Second).UnixMilli() {
		t.Fatalf("query protection changed publication or lost bound: %#v", claims)
	}
	spoof := &preparedResource{id: claims[1].ID, session: ticket.session}
	if err := spoof.Confirm(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("query lease became indefinite publication: %v", err)
	}
	// Another process sees the same reference, without a local engine cache.
	peer := reopenResourceEngine(t, e, resources)
	durable, _, err := peer.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil || len(durable.Resource.Claims) != 2 {
		t.Fatalf("query protection was only local: %#v %v", durable.Resource, err)
	}
	report, err := peer.RecoverPublications(ctx, func(context.Context, []byte) (bool, error) {
		t.Fatal("active query protection used publication resolver")
		return false, nil
	})
	if err != nil || len(report.Released) != 0 || len(queryClaims(t, ticket)) != 2 {
		t.Fatalf("active query protection was discharged: %#v %v", report, err)
	}
	cancel()
	if _, err := bound.Count(); !errors.Is(err, context.Canceled) {
		t.Fatalf("query escaped original request cancellation: %v", err)
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	claims = queryClaims(t, ticket)
	if len(claims) != 1 || claims[0].ID != ticket.id || claims[0].Phase != claimAccepted {
		t.Fatalf("query close released adopted publication: %#v", claims)
	}
}

func TestLiveOpenSearchExpiredQueryLeaseRecoversAfterProcessLoss(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("query-expiry-is-not-publication-expiry")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	bound, err := e.BindPublication(request, evidence)
	if err != nil {
		t.Fatal(err)
	}
	lease := bound.(*openSearchEngine).queryProtection
	if count, err := bound.Count(); err != nil || count != 1 {
		t.Fatalf("protected corpus unavailable: %d %v", count, err)
	}
	<-request.Done()
	if _, err := bound.Count(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("expired query view remained usable: %v", err)
	}
	// Do not close bound: recovery must use the durable deadline after a crash.
	peer := reopenResourceEngine(t, e, resources)
	report, err := peer.RecoverPublications(ctx, func(context.Context, []byte) (bool, error) {
		t.Fatal("query expiry consulted or released publication authority")
		return false, nil
	})
	if err != nil || len(report.Released) != 1 || len(report.Confirmed) != 0 {
		t.Fatalf("expired query did not recover: %#v %v", report, err)
	}
	claims := queryClaims(t, ticket)
	if len(claims) != 1 || claims[0].ID != ticket.id || claims[0].Phase != claimAccepted {
		t.Fatalf("query expiry released adopted preparation: %#v", claims)
	}
	if lease.until == 0 {
		t.Fatal("test did not bind a persistent deadline")
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := peer.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
}

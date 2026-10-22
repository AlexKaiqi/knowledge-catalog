package opensearchimpl

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func expirePublicationClaim(t *testing.T, ticket *preparedResource) {
	t.Helper()
	e := ticket.session.engine
	record, version, err := e.loadControlRecord(context.Background(), ticket.session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range record.Resource.Claims {
		if record.Resource.Claims[i].ID == ticket.id {
			record.Resource.Claims[i].Until = time.Now().Add(-time.Minute).UnixMilli()
			found = true
		}
	}
	if !found {
		t.Fatal("claim to expire was missing")
	}
	if _, err := e.putControlRecord(ticket.session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
}

func resourceAuthorityForTest(t *testing.T, e *openSearchEngine) ports.ResourceCoordinator {
	t.Helper()
	coordinator, err := OpenResources(Config{URL: e.base, Namespace: e.namespace, PrimaryShards: 1})
	if err != nil || coordinator == nil {
		t.Fatalf("no instance resource coordinator: %v", err)
	}
	t.Cleanup(func() { _ = coordinator.Close() })
	return coordinator
}

func TestLiveOpenSearchInstanceRecoversPublicationClaimsInUnopenedScopes(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, accepted := preparePublicationResource(t, e)
	engine, err := Open(Config{URL: e.base, Namespace: e.namespace, PrimaryShards: 1})(t.TempDir(), "kr://publication/another-scope")
	if err != nil {
		t.Fatal(err)
	}
	other := engine.(*openSearchEngine)
	other.http.Transport = resources
	t.Cleanup(func() { _ = other.Close() })
	_, rejected := preparePublicationResource(t, other)
	ctx := context.Background()
	for _, entry := range []struct {
		ticket *preparedResource
		proof  string
	}{{accepted, "accepted-proof"}, {rejected, "never-adopted-proof"}} {
		if err := entry.ticket.Admit(ctx, []byte(entry.proof)); err != nil {
			t.Fatal(err)
		}
		expirePublicationClaim(t, entry.ticket)
	}
	// A fresh instance authority has opened no query scope and no source demand.
	coordinator := resourceAuthorityForTest(t, e)
	calls := 0
	report, err := coordinator.RecoverPublications(ctx, func(_ context.Context, proof []byte) (bool, error) {
		calls++
		return string(proof) == "accepted-proof", nil
	})
	if err != nil || len(report.Confirmed) != 1 || len(report.Released) != 1 || len(report.Retained) != 0 || calls != 2 {
		t.Fatalf("instance recovery missed or mixed scopes: %#v calls=%d err=%v", report, calls, err)
	}
	for _, entry := range []struct {
		ticket *preparedResource
		count  int
	}{{accepted, 1}, {rejected, 0}} {
		record, _, err := entry.ticket.session.engine.loadControlRecord(ctx, entry.ticket.session.resourceID())
		if err != nil || len(record.Resource.Claims) != entry.count {
			t.Fatalf("wrong recovered references: %#v %v", record, err)
		}
		if entry.count == 1 && record.Resource.Claims[0].Phase != claimAccepted {
			t.Fatal("accepted reference not confirmed durably")
		}
		requireGenerationStatus(t, e, entry.ticket.session.physicalIndex, http.StatusOK)
	}
	if err := rejected.Admit(ctx, []byte("never-adopted-proof")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("released ticket resurrected publication: %v", err)
	}
	report, err = coordinator.RecoverPublications(ctx, func(context.Context, []byte) (bool, error) {
		t.Error("settled claim was resolved again")
		return false, nil
	})
	if err != nil || len(report.Confirmed)+len(report.Released)+len(report.Retained) != 0 {
		t.Fatalf("settlement was not durable: %#v %v", report, err)
	}
}

func TestLiveOpenSearchUnknownPublicationOutcomeRemainsProtected(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	if err := ticket.Admit(ctx, []byte("unknown-authority")); err != nil {
		t.Fatal(err)
	}
	expirePublicationClaim(t, ticket)
	coordinator := resourceAuthorityForTest(t, e)
	unknown := errors.New("actual authority unavailable")
	report, err := coordinator.RecoverPublications(ctx, func(context.Context, []byte) (bool, error) { return false, unknown })
	if !errors.Is(err, unknown) || len(report.Retained) != 1 || len(report.Released) != 0 {
		t.Fatalf("unknown publication was released: %#v %v", report, err)
	}
	if err := ticket.Cancel(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("unknown publication became cancellable: %v", err)
	}
	report, err = resourceAuthorityForTest(t, e).RecoverPublications(ctx, func(context.Context, []byte) (bool, error) { return true, nil })
	if err != nil || len(report.Confirmed) != 1 {
		t.Fatalf("reopened recovery did not settle known acceptance: %#v %v", report, err)
	}
}

func TestLiveOpenSearchActiveAdmissionIsNotFencedByAnotherExpiredClaim(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	meta, active := preparePublicationResource(t, e)
	ctx := context.Background()
	if err := active.Admit(ctx, []byte("still-publishing")); err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareResource(ctx, meta)
	if err != nil {
		t.Fatal(err)
	}
	expirePublicationClaim(t, prepared.(*preparedResource))
	report, err := resourceAuthorityForTest(t, e).RecoverPublications(ctx, func(context.Context, []byte) (bool, error) {
		t.Error("active publication was fenced")
		return false, nil
	})
	if err != nil || len(report.Released)+len(report.Confirmed)+len(report.Retained) != 0 {
		t.Fatalf("active admission was recovered early: %#v %v", report, err)
	}
	if err := active.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLiveOpenSearchRecoveryCannotRemoveConcurrentPublicationConfirmation(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	if err := ticket.Admit(ctx, []byte("same-evidence")); err != nil {
		t.Fatal(err)
	}
	expirePublicationClaim(t, ticket)
	report, err := resourceAuthorityForTest(t, e).RecoverPublications(ctx, func(context.Context, []byte) (bool, error) { return false, ticket.Confirm(ctx) })
	if err != nil || len(report.Confirmed) != 1 || len(report.Released) != 0 {
		t.Fatalf("concurrent confirmation was erased: %#v %v", report, err)
	}
	if err := ticket.Cancel(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("accepted reference was removed: %v", err)
	}
}

func TestLiveOpenSearchInstanceReclaimsExpiredCandidateWithoutQueryHandles(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "abandoned"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	defer session.Abort(errors.New("cleanup"))
	expireResource(t, e, session)
	report, err := resourceAuthorityForTest(t, e).RecoverResources(context.Background())
	if err != nil || len(report.Deleted) != 1 || report.Deleted[0] != session.physicalIndex {
		t.Fatalf("instance missed unopened expired candidate: %#v %v", report, err)
	}
	requireGenerationStatus(t, e, session.physicalIndex, http.StatusNotFound)
	if err := session.Append(publicationRecoveryDocs()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old builder regained write permission: %v", err)
	}
}

package opensearchimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func preparePublicationResource(t *testing.T, e *openSearchEngine) (ports.Meta, *preparedResource) {
	t.Helper()
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "fixed"}); err != nil {
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
	return meta, ticket.(*preparedResource)
}

func TestLiveOpenSearchPreparedResourceExcludesIncrementalMutation(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	meta, ticket := preparePublicationResource(t, e)
	peer := reopenResourceEngine(t, e, resources)
	if err := peer.Apply(publicationRecoveryDocs(), nil, ports.Meta{Basis: "changed"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("prepared generation allowed mutation: %v", err)
	}
	if actual, err := peer.LoadMeta(); err != nil || !reflect.DeepEqual(actual, meta) {
		t.Fatalf("rejected mutation changed ready basis: %#v %v", actual, err)
	}
	if err := ticket.Cancel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Cancel(context.Background()); err != nil {
		t.Fatalf("cancel was not idempotent: %v", err)
	}
	if err := peer.Apply(publicationRecoveryDocs(), nil, ports.Meta{Basis: "changed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.PrepareResource(context.Background(), meta); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old metadata acquired changed resource: %v", err)
	}
	if err := ticket.Admit(context.Background(), []byte("old-publication")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cancelled ticket admitted publication: %v", err)
	}
}

func TestLiveOpenSearchAdmittedResourceOutlivesPreparationLeaseAndReopen(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("authority-instance/ref/base/exact-intent")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	record, version, err := e.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil || len(record.Resource.Claims) != 1 {
		t.Fatalf("missing claim: %#v %v", record, err)
	}
	record.Resource.Claims[0].Until = time.Now().Add(-time.Hour).UnixMilli()
	if _, err := e.putControlRecord(ticket.session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	recovered := &preparedResource{id: ticket.id, session: &rebuildSession{engine: peer, physicalIndex: ticket.session.physicalIndex, generation: ticket.session.generation}}
	if err := recovered.Cancel(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("timeout cancelled admitted publication: %v", err)
	}
	if err := peer.Apply(publicationRecoveryDocs(), nil, ports.Meta{Basis: "changed"}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("expired admission allowed mutation: %v", err)
	}
	if err := recovered.Admit(ctx, evidence); err != nil {
		t.Fatalf("same evidence was not retryable: %v", err)
	}
	if err := recovered.Admit(ctx, []byte("foreign-intent")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("same ticket accepted different evidence: %v", err)
	}
	if err := recovered.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	record, _, err = peer.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil || record.Resource.Claims[0].Phase != claimAccepted || string(record.Resource.Claims[0].Evidence) != string(evidence) {
		t.Fatalf("accepted reference was not durable: %#v %v", record, err)
	}
}

func TestLiveOpenSearchExpiredPreparedTicketCannotAcquirePublication(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	record, version, err := e.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	record.Resource.Claims[0].Until = time.Now().Add(-time.Minute).UnixMilli()
	if _, err := e.putControlRecord(ticket.session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Admit(ctx, []byte("too-late")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("expired preparation admitted publication: %v", err)
	}
	peer := reopenResourceEngine(t, e, resources)
	if err := peer.Apply(publicationRecoveryDocs(), nil, ports.Meta{Basis: "changed"}); err != nil {
		t.Fatalf("expired preparation prevented safe update: %v", err)
	}
	if err := ticket.Admit(ctx, []byte("still-too-late")); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("pruned old ticket reappeared: %v", err)
	}
}

type loseResourceAdmission struct {
	next http.RoundTripper
	path string
	lost atomic.Bool
}

func (l *loseResourceAdmission) RoundTrip(request *http.Request) (*http.Response, error) {
	admitting := false
	if request.Method == http.MethodPut && request.URL.Path == l.path && request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		var record controlDoc
		err = json.NewDecoder(body).Decode(&record)
		_ = body.Close()
		if err != nil {
			return nil, err
		}
		admitting = record.Resource != nil && len(record.Resource.Claims) == 1 && record.Resource.Claims[0].Phase == claimAdmitted
	}
	response, err := l.next.RoundTrip(request)
	if err == nil && admitting && response.StatusCode < 300 && l.lost.CompareAndSwap(false, true) {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, fmt.Errorf("injected response loss after actual resource admission")
	}
	return response, err
}

func TestLiveOpenSearchLostResourceAdmissionResponseRetainsDurableProtection(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	e.http.Transport = &loseResourceAdmission{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + ticket.session.resourceID()}
	ctx := context.Background()
	if err := ticket.Admit(ctx, []byte("authority-proof")); err == nil {
		t.Fatal("admission response was not lost")
	}
	peer := reopenResourceEngine(t, e, resources)
	record, _, err := peer.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil || len(record.Resource.Claims) != 1 || record.Resource.Claims[0].Phase != claimAdmitted {
		t.Fatalf("lost response erased admission: %#v %v", record, err)
	}
	if err := ticket.Cancel(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("unknown admission was released: %v", err)
	}
	requireGenerationStatus(t, peer, ticket.session.physicalIndex, http.StatusOK)
}

func TestLiveOpenSearchLegacyResourceLedgerUpgradesBeforeIssuingClaims(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "fixed"}); err != nil {
		t.Fatal(err)
	}
	meta, err := e.LoadMeta()
	if err != nil {
		t.Fatal(err)
	}
	session := &rebuildSession{engine: e, physicalIndex: e.prefix + "-g-" + meta.Generation, generation: meta.Generation}
	ctx := context.Background()
	record, version, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	record.Resource.Version = 1
	record.Resource.QueryLeases = 0
	if _, err := e.putControlRecord(session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	legacy, legacyVersion, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := e.PrepareResource(ctx, meta)
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()
	after, _, err := e.loadControlRecord(ctx, session.resourceID())
	if err != nil || after.Resource.Version != resourceRecordVersion || after.Resource.QueryLeases != 0 || len(after.Resource.Claims) != 1 {
		t.Fatalf("claims were issued without ledger version upgrade: %#v %v", after, err)
	}
	if _, err := e.putControlRecord(session.resourceID(), legacy, legacyVersion); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("pre-upgrade writer overwrote publication claims: %v", err)
	}
	if err := ticket.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
}

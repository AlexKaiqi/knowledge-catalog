package opensearchimpl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func TestLiveOpenSearchPublicationRepairUsesFreshCanonicalGeneration(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("repair-fixed-publication")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	calls := 0
	prepare := func(context.Context) (ports.Meta, error) {
		calls++
		if err := peer.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "canonical-repair"}); err != nil {
			return ports.Meta{}, err
		}
		return peer.LoadMeta()
	}
	accept := func(apply func() error) error { return apply() }
	if err := peer.RepairPublication(ctx, evidence, ports.Meta{}, prepare, accept); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("healthy binding triggered preparation")
	}
	if status, _, err := e.do(http.MethodDelete, "/"+ticket.session.physicalIndex, nil); err != nil || status >= 300 {
		t.Fatalf("remove physical corpus: %d %v", status, err)
	}
	if _, err := peer.BindPublication(ctx, evidence); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing physical corpus remained queryable: %v", err)
	}
	if err := peer.RepairPublication(ctx, evidence, ports.Meta{}, prepare, accept); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("canonical preparations: %d", calls)
	}
	bound, err := reopenResourceEngine(t, e, resources).BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	meta, err := bound.LoadMeta()
	if err != nil || meta.Generation == ticket.session.generation || meta.Basis != "canonical-repair" {
		t.Fatalf("repair did not install fresh canonical generation: %#v %v", meta, err)
	}
	if count, err := bound.Count(); err != nil || count != 1 {
		t.Fatalf("repaired corpus: %d %v", count, err)
	}
}

func TestLiveOpenSearchPublicationRepairRetainsUnknownAdmissionAndProvesLostAck(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("repair-missing-binding")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	id := e.publicationBindingID(evidence)
	if status, _, err := e.do(http.MethodDelete, "/"+e.controlIndexPath()+"/_doc/"+id, nil); err != nil || status >= 300 {
		t.Fatalf("remove binding: %d %v", status, err)
	}
	prepare := func(context.Context) (ports.Meta, error) {
		if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "repair-ack"}); err != nil {
			return ports.Meta{}, err
		}
		return e.LoadMeta()
	}
	denied := kernel.Fail(kernel.ErrPreconditionFailed, "publication retired during preparation")
	if err := e.RepairPublication(ctx, evidence, ports.Meta{}, prepare, func(func() error) error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("authority rejection lost: %v", err)
	}
	if _, err := e.BindPublication(ctx, evidence); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("rejected repair installed binding: %v", err)
	}
	lost := &losePublicationBindingResponse{next: e.http.Transport, path: "/" + e.controlIndexPath() + "/_doc/" + id}
	e.http.Transport = lost
	if err := e.RepairPublication(ctx, evidence, ports.Meta{}, prepare, func(apply func() error) error { return apply() }); err != nil {
		t.Fatal(err)
	}
	if !lost.lost.Load() {
		t.Fatal("repair acknowledgement was not lost")
	}
	bound, err := e.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if count, err := bound.Count(); err != nil || count != 1 {
		t.Fatalf("lost acknowledgement repair: %d %v", count, err)
	}
}

func TestLiveOpenSearchPublicationRepairRejectsUnknownResourceProtection(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("unknown-resource-protection")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	resource, version, err := e.loadControlRecord(ctx, ticket.session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	resource.Resource.Claims[0].Phase = "UNKNOWN_FUTURE_PROTOCOL"
	if _, err := e.putControlRecordContext(ctx, ticket.session.resourceID(), resource, version); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = e.RepairPublication(ctx, evidence, ports.Meta{}, func(context.Context) (ports.Meta, error) { calls++; return ports.Meta{}, nil }, func(apply func() error) error { return apply() })
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || calls != 0 {
		t.Fatalf("unknown protection entered canonical replacement: calls=%d err=%v", calls, err)
	}
	mapping, _, err := e.loadControlRecord(ctx, e.publicationBindingID(evidence))
	if err != nil || mapping.Generation != ticket.session.generation {
		t.Fatalf("unknown protection changed original mapping: %#v %v", mapping, err)
	}
	requireGenerationStatus(t, e, ticket.session.physicalIndex, http.StatusOK)
}

// A failed readiness probe is not evidence that the physical corpus is absent.
type unavailablePublicationMedium struct {
	next http.RoundTripper
	path string
}

func (u unavailablePublicationMedium) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodHead && request.URL.Path == u.path {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	}
	return u.next.RoundTrip(request)
}

func TestLiveOpenSearchPublicationRepairDoesNotBuildDuringReadinessOutage(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	_, ticket := preparePublicationResource(t, e)
	ctx := context.Background()
	evidence := []byte("unavailable-is-not-missing")
	if err := ticket.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	e.http.Transport = unavailablePublicationMedium{next: resources, path: "/" + ticket.session.physicalIndex}
	calls := 0
	err := e.RepairPublication(ctx, evidence, ports.Meta{}, func(context.Context) (ports.Meta, error) { calls++; return ports.Meta{}, nil }, func(apply func() error) error { return apply() })
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || calls != 0 {
		t.Fatalf("readiness outage triggered replacement: calls=%d err=%v", calls, err)
	}
	e.http.Transport = resources
	mapping, _, err := e.loadControlRecord(ctx, e.publicationBindingID(evidence))
	if err != nil || mapping.Generation != ticket.session.generation {
		t.Fatalf("readiness outage changed binding: %#v %v", mapping, err)
	}
	requireGenerationStatus(t, e, ticket.session.physicalIndex, http.StatusOK)
}

func TestLiveOpenSearchPublicationRepairReplacesReadableIncompatibleCompiler(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	ctx := context.Background()
	oldMeta := ports.Meta{Basis: "fixed", PhysicalDigest: "old-compiler-digest", ProviderRevision: e.ProviderRevision()}
	if err := e.Rebuild(publicationRecoveryDocs(), oldMeta); err != nil {
		t.Fatal(err)
	}
	oldMeta, err := e.LoadMeta()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := e.PrepareResource(ctx, oldMeta)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []byte("immutable-source-current-compiler")
	if err := prepared.Admit(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Confirm(ctx); err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	retained, err := e.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Close()
	desired := ports.Meta{Basis: "fixed", PhysicalDigest: e.PhysicalDigest(), ProviderRevision: e.ProviderRevision()}
	calls := 0
	prepare := func(context.Context) (ports.Meta, error) {
		calls++
		if err := e.Rebuild(publicationRecoveryDocs(), desired); err != nil {
			return ports.Meta{}, err
		}
		return e.LoadMeta()
	}
	if err := e.RepairPublication(ctx, evidence, desired, prepare, func(apply func() error) error { return apply() }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("readable old compiler bypassed repair: preparations=%d", calls)
	}
	report, err := e.ReconcilePublicationReferences(ctx, func(context.Context, []byte) (bool, error) { return true, nil })
	oldTicket := prepared.(*preparedResource)
	expectedReleased := oldTicket.session.physicalIndex + "/" + oldTicket.id
	if err != nil || !slices.Contains(report.Released, expectedReleased) {
		t.Fatalf("repaired publication kept obsolete adopted root: %#v %v", report, err)
	}
	if count, err := retained.Count(); err != nil || count != 1 {
		t.Fatalf("releasing replaced publication invalidated admitted query: %d %v", count, err)
	}

	current, err := e.BindPublication(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	actual, err := current.LoadMeta()
	if err != nil || actual.PhysicalDigest != desired.PhysicalDigest || actual.Generation == oldMeta.Generation {
		t.Fatalf("compiler repair reused incompatible generation: %#v %v", actual, err)
	}
	kept, err := retained.LoadMeta()
	if err != nil || kept.Generation != oldMeta.Generation || kept.PhysicalDigest != oldMeta.PhysicalDigest {
		t.Fatalf("repair changed already admitted query: %#v %v", kept, err)
	}
	if err := e.RepairPublication(ctx, evidence, desired, prepare, func(apply func() error) error { return apply() }); err != nil || calls != 1 {
		t.Fatalf("compatible corpus rebuilt again: calls=%d %v", calls, err)
	}
	incompatible := desired
	incompatible.PhysicalDigest = "unavailable-compiler"
	err = e.RepairPublication(ctx, evidence, incompatible, func(context.Context) (ports.Meta, error) { return desired, nil }, func(func() error) error { t.Fatal("incompatible preparation reached binding acceptance"); return nil })
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("incorrect preparation accepted: %v", err)
	}
}

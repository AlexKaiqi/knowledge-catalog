package opensearchimpl

import (
	"context"
	"testing"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func demandTestMeta() ports.Meta { return ports.Meta{Basis: "demand-basis"} }

// A confirmed consumer rebuilds through the public provider, so demand must be
// durable before physical work and observable by a peer process afterwards.
func TestLiveOpenSearchRebuildRegistersConfirmedDemand(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.ensureControlIndex(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, version, err := e.loadScopeDemand(ctx); err != nil || version != nil {
		t.Fatalf("fresh scope already has demand: %v", err)
	}
	session, err := e.BeginRebuild(demandTestMeta())
	if err != nil {
		t.Fatal(err)
	}
	demand, version, err := e.loadScopeDemand(ctx)
	if err != nil || version == nil {
		t.Fatalf("rebuild did not register confirmed demand: %v", err)
	}
	if demand.Phase != scopeActive || !validGeneration(demand.Epoch) {
		t.Fatalf("rebuild registered unusable demand: %#v", demand)
	}
	// Abort reports the build error it was given; the candidate itself is gone.
	if err := session.Abort(kernel.Fail(kernel.ErrTemporaryUnavailable, "test aborts the candidate")); err == nil {
		t.Fatal("abort silently reported success for a failed candidate")
	}
	// Registration reflects a real consumer: an aborted candidate does not
	// retire the scope, because the consumer itself has not left.
	if err := e.checkScopeDemand(ctx, demand.Epoch); err != nil {
		t.Fatalf("aborted candidate retired live demand: %v", err)
	}
	peer := reopenResourceEngine(t, e, resources)
	if err := peer.checkScopeDemand(ctx, demand.Epoch); err != nil {
		t.Fatalf("peer process lost durable demand: %v", err)
	}
}

// Retirement is the only proof that stop is real: it must block new physical
// work and stay durable across a reopen.
func TestLiveOpenSearchRetiredDemandBlocksRebuildUntilReactivated(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.ensureControlIndex(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := e.activateScopeDemand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.RetireDemand(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.RetireDemand(ctx); err != nil {
		t.Fatalf("repeated stop was not idempotent: %v", err)
	}
	if _, err := e.BeginRebuild(demandTestMeta()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("retired demand still rebuilt physical state: %v", err)
	}
	if err := e.checkScopeDemand(ctx, first.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("retired epoch still admitted a writer: %v", err)
	}
	peer := reopenResourceEngine(t, e, resources)
	if _, err := peer.BeginRebuild(demandTestMeta()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("reopened process ignored durable retirement: %v", err)
	}
	// Background admission never revives a stopped scope; only an explicit
	// consumer returning may reopen it, and it must use a new epoch.
	if err := e.enterDemand(ctx); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("background admission revived a retired scope: %v", err)
	}
	if _, err := e.activateScopeDemand(ctx); err != nil {
		t.Fatal(err)
	}
	second, version, err := e.loadScopeDemand(ctx)
	if err != nil || version == nil {
		t.Fatal(err)
	}
	if second.Phase != scopeActive || second.Epoch == first.Epoch {
		t.Fatalf("reactivation reused the retired epoch: %#v", second)
	}
	if err := e.checkScopeDemand(ctx, second.Epoch); err != nil {
		t.Fatal(err)
	}
}

// An unmigrated legacy namespace has no exclusive owner, so the gate must stay
// silent rather than invent retirement it cannot prove.
func TestScopeDemandGateIsSilentWithoutOwnedNamespace(t *testing.T) {
	e := scopeDemandEngine(t)
	e.namespace = ""
	if err := e.enterDemand(context.Background()); err != nil {
		t.Fatalf("legacy namespace fabricated retirement: %v", err)
	}
	if err := e.RetireDemand(context.Background()); err != nil {
		t.Fatalf("legacy namespace fabricated a stop: %v", err)
	}
}

// Retirement observed by a stale process must not close demand that a newer
// owner activated, and an unknown authority is never permission to build.
func TestScopeDemandGateRejectsObsoleteAndUnknownAuthority(t *testing.T) {
	e := scopeDemandEngine(t)
	ctx := context.Background()
	first, err := e.activateScopeDemand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	peer := e.resourceScope(e.repository)
	if err := peer.retireScopeDemand(ctx, first.Epoch); err != nil {
		t.Fatal(err)
	}
	second, err := peer.activateScopeDemand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.enterDemand(ctx); err != nil {
		t.Fatalf("stale process saw retirement after reactivation: %v", err)
	}
	if second.Epoch == first.Epoch {
		t.Fatal("reactivation reused a retired epoch")
	}
}

package opensearchimpl

import (
	"context"
	"testing"

	"kc/kernel"
)

func TestLiveOpenSearchScopeDemandSurvivesColdReopenAndRejectsOldEpoch(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.ensureControlIndex(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := e.activateScopeDemand(ctx)
	if err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	if err := peer.checkScopeDemand(ctx, first.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := peer.retireScopeDemand(ctx, first.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := e.checkScopeDemand(ctx, first.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old process retained admission after durable retirement: %v", err)
	}
	newOwner := reopenResourceEngine(t, e, resources)
	second, err := newOwner.activateScopeDemand(ctx)
	if err != nil || first.Epoch == second.Epoch {
		t.Fatalf("reactivation did not replace writer epoch: %#v %v", second, err)
	}
	if err := peer.retireScopeDemand(ctx, first.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("obsolete owner retired new demand: %v", err)
	}
	if err := e.checkScopeDemand(ctx, first.Epoch); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old writer epoch was revived: %v", err)
	}
	if err := e.checkScopeDemand(ctx, second.Epoch); err != nil {
		t.Fatal(err)
	}
}

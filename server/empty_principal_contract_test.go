package server

import (
	"context"
	"testing"

	"kc/identity"
	"kc/kernel"
)

func TestDebtSnapshotAcquireRejectsMissingPrincipalBeforeHome(t *testing.T) {
	for _, principal := range []string{"", " \t "} {
		t.Run(principal, func(t *testing.T) {
			opened := false
			facade := &httpFacade{openHome: func() (*Home, error) {
				opened = true
				return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "Home must not open for an absent principal")
			}}
			_, release, err := facade.acquireSnapshotResources(context.Background(), identity.HTTPIdentity{Principal: principal}, "inventory-show")
			if release != nil {
				release()
			}
			if opened || kernel.CodeOf(err) != kernel.ErrUnauthenticated {
				t.Fatalf("empty Snapshot caller reached Home: opened=%v error=%v", opened, err)
			}
		})
	}
}

func TestDebtSnapshotInventoryCannotEvaluateAsOwner(t *testing.T) {
	opened := &Home{Dir: t.TempDir()}
	facade := &httpFacade{readHome: opened, workersStarted: true}
	_, release, err := facade.acquireSnapshotResources(context.Background(), identity.HTTPIdentity{}, "inventory-show")
	if release != nil {
		release()
	}
	if kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("Snapshot inventory accepted an absent principal: %v", err)
	}
	if opened.Journal != nil {
		t.Fatal("Snapshot omission adopted an owner journal before rejection")
	}
}

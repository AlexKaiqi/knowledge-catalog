package control

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	fixtures "kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

type recoveryMaintenanceIndex struct {
	MaintenanceIndex
	ensures int
	calls   int
	err     error
}

func (i *recoveryMaintenanceIndex) RecoverResources(context.Context) error {
	i.calls++
	return i.err
}

func (i *recoveryMaintenanceIndex) Ensure(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexSync, error) {
	i.ensures++
	return i.MaintenanceIndex.Ensure(ctx, repo, commit)
}

func TestResourceRecoveryRunsWithReadyLedgerButNotReadOnlyOrScopedReconcile(t *testing.T) {
	repo, _, head := fixtures.CommittedSearchablePolicy(t)
	idx := storageLossIndex(t, &storageLossEngine{})
	controller := storageLossController(t, idx, filepath.Join(t.TempDir(), "controller.db"), repo)
	if err := controller.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertStorageLossReadyLedger(t, controller, repo.ID(), head)
	recovery := &recoveryMaintenanceIndex{MaintenanceIndex: idx}
	controller.index = recovery
	if err := controller.Reconcile(context.Background()); err != nil || recovery.calls != 0 {
		t.Fatalf("read-only reconcile ran physical recovery: %v, calls=%d", err, recovery.calls)
	}
	if err := controller.CatchUpRepository(context.Background(), repo.ID()); err != nil || recovery.calls != 0 {
		t.Fatalf("scoped catch-up ran global physical recovery: %v, calls=%d", err, recovery.calls)
	}
	if err := controller.CatchUp(context.Background()); err != nil || recovery.calls != 1 || recovery.ensures != 0 {
		t.Fatalf("READY target suppressed resource recovery or forced rebuild: %v, recoveries=%d ensures=%d", err, recovery.calls, recovery.ensures)
	}
	failed := errors.New("unresolved physical protection")
	recovery.err = failed
	if err := controller.CatchUp(context.Background()); !errors.Is(err, failed) {
		t.Fatalf("physical recovery diagnostic hidden: %v", err)
	}
	assertStorageLossReadyLedger(t, controller, repo.ID(), head)
}

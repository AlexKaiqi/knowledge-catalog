package control_test

import (
	"context"
	"errors"
	fixtures "kc/declarativeindex/internal/testfixture"
	"path/filepath"
	"testing"
	"time"

	control "kc/declarativeindex/control"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

// blockingMaintenanceIndex holds Ensure until the worker context is cancelled,
// standing in for a rebuild that is still reading objects at shutdown.
type blockingMaintenanceIndex struct {
	started chan struct{}
	result  chan error
}

func (b *blockingMaintenanceIndex) Ensure(ctx context.Context, _ objectmodel.Repository, _ kernel.CommitID) (index.IndexSync, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	b.result <- ctx.Err()
	return index.IndexSync{}, ctx.Err()
}

func (b *blockingMaintenanceIndex) EnsureAt(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID) (index.IndexSync, error) {
	return b.Ensure(ctx, repo, commit)
}

func (*blockingMaintenanceIndex) Describe(objectmodel.Repository) (index.IndexDescriptor, error) {
	return index.IndexDescriptor{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "projection is not built")
}

func (*blockingMaintenanceIndex) HasStateBindings(objectmodel.Repository, kernel.CommitID) (bool, error) {
	return false, nil
}

func (*blockingMaintenanceIndex) StateProjectionReady(context.Context, objectmodel.Repository, kernel.CommitID) (bool, error) {
	return true, nil
}

func (*blockingMaintenanceIndex) RefreshState(context.Context, objectmodel.Repository, kernel.CommitID, index.ObservationSource) (index.StateSync, error) {
	return index.StateSync{}, nil
}

func (*blockingMaintenanceIndex) RefreshStateObjects(context.Context, objectmodel.Repository, kernel.CommitID, index.ObservationSource, []objectmodel.ObjectID) (index.StateSync, error) {
	return index.StateSync{}, nil
}

func TestProjectionCloseCancelsInFlightEnsureAndKeepsTargetPending(t *testing.T) {
	repo, _, head := fixtures.CommittedSearchablePolicy(t)
	idx := &blockingMaintenanceIndex{started: make(chan struct{}, 1), result: make(chan error, 4)}
	controller, err := control.NewController(idx, control.NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
		func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil })
	if err != nil {
		t.Fatal(err)
	}
	controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repo.ID()}, nil })
	t.Cleanup(controller.Close)
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-idx.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background maintenance never reached Ensure")
	}
	closed := make(chan struct{})
	go func() { controller.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return while Ensure was in flight")
	}
	if got := <-idx.result; !errors.Is(got, context.Canceled) {
		t.Fatalf("in-flight Ensure returned %v, want context.Canceled", got)
	}
	targets, err := controller.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].DesiredCommit != head || targets[0].Status != index.TargetPending || targets[0].LastError != "" {
		t.Fatalf("shutdown must leave the durable target pending for the next start: %#v", targets)
	}
}

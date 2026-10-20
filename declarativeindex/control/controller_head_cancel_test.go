package control_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	control "kc/declarativeindex/control"
	fixtures "kc/declarativeindex/internal/testfixture"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type blockedHeadRepository struct {
	objectmodel.Repository
	ctx                context.Context
	blockAfter         int32
	succeedAfterCancel bool
	calls              *atomic.Int32
	entered            chan struct{}
	cleanup            <-chan struct{}
}

func (r *blockedHeadRepository) WithContext(ctx context.Context) objectmodel.Repository {
	view := *r
	view.ctx = ctx
	return &view
}

func (r *blockedHeadRepository) Head(ref string) (kernel.CommitID, error) {
	if r.calls.Add(1) < r.blockAfter {
		return r.Repository.Head(ref)
	}
	r.entered <- struct{}{}
	select {
	case <-r.ctx.Done():
		if r.succeedAfterCancel {
			return r.Repository.Head(ref)
		}
		return "", r.ctx.Err()
	case <-r.cleanup:
		return "", errors.New("test cleanup released an uncanceled HEAD")
	}
}

func (r *blockedHeadRepository) SchemaObjectIDs(kernel.CommitID) ([]objectmodel.ObjectID, error) {
	_, err := r.Head(snapshot.DefaultRef)
	return nil, err
}

type repositoryReadingMaintenanceIndex struct {
	blockingMaintenanceIndex
}

func (*repositoryReadingMaintenanceIndex) Ensure(_ context.Context, repo objectmodel.Repository, _ kernel.CommitID) (index.IndexSync, error) {
	_, err := repo.Head(snapshot.DefaultRef)
	return index.IndexSync{}, err
}

func TestProjectionCancellationBindsHeadAndEnsureRepository(t *testing.T) {
	for _, stage := range []struct {
		name               string
		blockAfter         int32
		succeedAfterCancel bool
		prepare            bool
	}{
		{name: "reconcile HEAD", blockAfter: 1},
		{name: "Ensure repository read", blockAfter: 2},
		{name: "Ensure success after cancellation", blockAfter: 2, succeedAfterCancel: true},
		{name: "prepare schema read", blockAfter: 1, prepare: true},
	} {
		t.Run(stage.name, func(t *testing.T) {
			inner, _, head := fixtures.CommittedSearchablePolicy(t)
			cleanup := make(chan struct{})
			repo := &blockedHeadRepository{
				Repository: inner, ctx: context.Background(), blockAfter: stage.blockAfter,
				succeedAfterCancel: stage.succeedAfterCancel,
				calls:              &atomic.Int32{}, entered: make(chan struct{}, 1), cleanup: cleanup,
			}
			controller, err := control.NewController(&repositoryReadingMaintenanceIndex{},
				control.NewTargetStore(filepath.Join(t.TempDir(), "controller.db")),
				func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil })
			if err != nil {
				t.Fatal(err)
			}
			controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repo.ID()}, nil })
			t.Cleanup(controller.Close)
			if err := controller.Desire(repo.ID(), head); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				if stage.prepare {
					done <- controller.PrepareServingBasis(ctx, repo, head)
				} else {
					done <- controller.CatchUp(ctx)
				}
			}()
			t.Cleanup(func() { close(cleanup); <-finished })
			select {
			case <-repo.entered:
			case err := <-done:
				t.Fatalf("maintenance returned before blocked HEAD: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("maintenance did not reach blocked HEAD")
			}
			before, err := controller.Targets()
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("maintenance returned %v, want context.Canceled", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation did not interrupt the repository HEAD call")
			}
			after, err := controller.Targets()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("canceled attempt changed durable target: before=%#v after=%#v error=%v", before, after, err)
			}
		})
	}
}

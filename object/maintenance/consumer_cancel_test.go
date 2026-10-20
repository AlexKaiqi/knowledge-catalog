package maintenance_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type blockedConsumerHeadRepository struct {
	objectmodel.Repository
	ctx                context.Context
	blockAfter         int32
	succeedAfterCancel bool
	calls              *atomic.Int32
	entered            chan struct{}
	cleanup            <-chan struct{}
}

func (r *blockedConsumerHeadRepository) WithContext(ctx context.Context) objectmodel.Repository {
	view := *r
	view.ctx = ctx
	return &view
}

func (r *blockedConsumerHeadRepository) Head(ref string) (kernel.CommitID, error) {
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

func TestSnapshotConsumerCancellationKeepsProgressAndBoundRepository(t *testing.T) {
	for _, stage := range []struct {
		name               string
		blockAfter         int32
		succeedAfterCancel bool
	}{
		{name: "reconcile HEAD", blockAfter: 1},
		{name: "apply HEAD", blockAfter: 2},
		{name: "consumer repository read", blockAfter: 3},
		{name: "consumer success after cancellation", blockAfter: 3, succeedAfterCancel: true},
	} {
		t.Run(stage.name, func(t *testing.T) {
			inner, _, head := committedConsumerPolicy(t)
			cleanup := make(chan struct{})
			repo := &blockedConsumerHeadRepository{
				Repository: inner, ctx: context.Background(), blockAfter: stage.blockAfter,
				succeedAfterCancel: stage.succeedAfterCancel,
				calls:              &atomic.Int32{}, entered: make(chan struct{}, 1), cleanup: cleanup,
			}
			coordinator := newConsumerCoordinator(t, filepath.Join(t.TempDir(), "controller.db"), repo)
			consumer := &testSnapshotConsumer{id: "cancel/v1", reconcile: func(_ context.Context, repo objectmodel.Repository, _ kernel.CommitID) error {
				_, err := repo.Head(snapshot.DefaultRef)
				return err
			}}
			if err := coordinator.RegisterConsumer(consumer); err != nil {
				t.Fatal(err)
			}
			if err := coordinator.Desire(repo.ID(), head); err != nil {
				t.Fatal(err)
			}
			before, err := coordinator.ConsumerTargets()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				done <- coordinator.CatchUp(ctx)
			}()
			t.Cleanup(func() { close(cleanup); <-finished })
			select {
			case <-repo.entered:
			case err := <-done:
				t.Fatalf("consumer returned before blocked HEAD: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("consumer did not reach blocked HEAD")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("consumer returned %v, want context.Canceled", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation did not interrupt the consumer repository HEAD call")
			}
			after, err := coordinator.ConsumerTargets()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("canceled consumer changed progress: before=%#v after=%#v error=%v", before, after, err)
			}
		})
	}
}

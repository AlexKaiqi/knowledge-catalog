package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/cache"
)

type blockedWarmRepository struct {
	*repository
	ctx       context.Context
	operation string
	entered   chan struct{}
	cleanup   <-chan struct{}
}

func (r *blockedWarmRepository) WithContext(ctx context.Context) objectmodel.Repository {
	view := *r
	view.ctx = ctx
	return &view
}

func (r *blockedWarmRepository) wait(operation string) error {
	if r.operation != operation {
		return nil
	}
	r.entered <- struct{}{}
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case <-r.cleanup:
		return errors.New("test cleanup released an uncanceled warm read")
	}
}

func (r *blockedWarmRepository) ObjectIDsPage(commit kernel.CommitID, limit int, continuation string) (objectmodel.ObjectIDPage, error) {
	if err := r.wait("identity page"); err != nil {
		return objectmodel.ObjectIDPage{}, err
	}
	return r.repository.ObjectIDsPage(commit, limit, continuation)
}

func (r *blockedWarmRepository) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	if err := r.wait("batch read"); err != nil {
		return nil, err
	}
	return r.repository.ReadMany(ids, commit)
}

func (r *blockedWarmRepository) ReadAddress(address objectmodel.Address, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	if err := r.wait("address read"); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return r.repository.ReadAddress(address, commit)
}

func TestWarmCancellationInterruptsAuthorityReadsWithoutFillingCache(t *testing.T) {
	for _, operation := range []string{"identity page", "batch read", "address read"} {
		t.Run(operation, func(t *testing.T) {
			inner := repo("kr://cache/cancel")
			c := newCache(t, cache.Config{ColdStart: true, WarmLimit: 2})
			switch operation {
			case "batch read":
				read(t, c, inner, "c1", "a")
			case "address read":
				if _, err := c.ReadAddress(inner, "c1", objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "a"}); err != nil {
					t.Fatal(err)
				}
			}
			before := c.Stats()
			cleanup := make(chan struct{})
			bound := &blockedWarmRepository{
				repository: inner, ctx: context.Background(), operation: operation,
				entered: make(chan struct{}, 1), cleanup: cleanup,
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				done <- c.Warm(ctx, bound, "c2")
			}()
			t.Cleanup(func() { close(cleanup); <-finished })
			select {
			case <-bound.entered:
			case err := <-done:
				t.Fatalf("warm returned before blocked %s: %v", operation, err)
			case <-time.After(3 * time.Second):
				t.Fatalf("warm did not reach %s", operation)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("warm returned %v, want context.Canceled", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("cancellation did not interrupt %s", operation)
			}
			after := c.Stats()
			if after.Entries != before.Entries || after.WarmMarkers != before.WarmMarkers || after.WarmObjects != before.WarmObjects {
				t.Fatalf("canceled warm changed retained state: before=%+v after=%+v", before, after)
			}
			if err := c.Warm(context.Background(), inner, "c2"); err != nil {
				t.Fatalf("warm could not retry after cancellation: %v", err)
			}
			if c.Stats().WarmObjects <= before.WarmObjects {
				t.Fatal("canceled warm incorrectly marked the basis complete")
			}
		})
	}
}

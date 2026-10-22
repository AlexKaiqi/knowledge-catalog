package reader_test

import (
	"context"
	"errors"
	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func retainedHistoryCheckpoint() objectmodel.HistoryCheckpoint {
	basis := retainedBasis()
	progress := &objectmodel.HistoryScanProgress{Algorithm: objectmodel.LocalStateAlgorithm, Object: "table/orders", Repository: "kr://trade", Binding: "authority-a", Start: "a1", Position: "older", Pending: objectmodel.ObjectRevision{Commit: "older", Status: objectmodel.StatusResolved, Digest: "body"}, State: "local-state"}
	return objectmodel.HistoryCheckpoint{Version: 1, Basis: basis, Open: map[kernel.RepositoryID]*objectmodel.HistoryScanProgress{"kr://trade": progress}}
}

func TestHistoryCheckpointSurvivesReopenWithIndependentReadMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordinates.db")
	ctx := context.Background()
	store, err := reader.NewHistoryCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := retainedHistoryCheckpoint()
	reference, err := store.Retain(ctx, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	basisStore, err := reader.NewReadBasisStore(path)
	if err != nil {
		t.Fatal(err)
	}
	basisReference, err := basisStore.Retain(ctx, checkpoint.Basis)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := reader.NewHistoryCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Resolve(ctx, reference)
	if err != nil || !reflect.DeepEqual(got, checkpoint) {
		t.Fatalf("checkpoint lost fixed source or unfinished segment: %#v %v", got, err)
	}
	got.Open["kr://trade"].Position = "changed"
	again, err := reopened.Resolve(ctx, reference)
	if err != nil || again.Open["kr://trade"].Position != "older" {
		t.Fatalf("mutable returned progress changed durable record: %#v %v", again, err)
	}
	if _, err := reopened.Resolve(ctx, basisReference); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("read basis accepted as history progress: %v", err)
	}
	if _, err := basisStore.Resolve(ctx, reference); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("history progress accepted as read basis: %v", err)
	}
	next := retainedHistoryCheckpoint()
	next.Open["kr://trade"].Position = "oldest"
	next.Open["kr://trade"].Pending.Commit = "oldest"
	newer, err := store.Retain(ctx, next)
	if err != nil || newer == reference {
		t.Fatalf("advanced scan did not change continuation: %s %v", newer, err)
	}
	old, err := store.Resolve(ctx, reference)
	if err != nil || old.Open["kr://trade"].Position != "older" {
		t.Fatal("new page overwrote original page")
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("object-history-checkpoints-v1")).Put([]byte(reference), []byte(`{"version":1}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, reference); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corrupt continuation accepted: %v", err)
	}
	if _, err := store.Retain(ctx, checkpoint); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corrupt progress silently replaced: %v", err)
	}
	if _, err := basisStore.Resolve(ctx, basisReference); err != nil {
		t.Fatalf("history corruption damaged independent read basis: %v", err)
	}
}

func TestHistoryCheckpointMissingInvalidAndCanceledDoNotCreateStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	store, err := reader.NewHistoryCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	valid := retainedHistoryCheckpoint()
	reference := "hs1_" + string(kernel.CanonicalDigest(valid))
	if _, err := store.Resolve(context.Background(), reference); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("missing record followed latest: %v", err)
	}
	invalid := retainedHistoryCheckpoint()
	invalid.Open["kr://trade"].Binding = "rebound"
	if _, err := store.Retain(context.Background(), invalid); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("source substitution persisted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Retain(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed metadata operation created an authority: %v", err)
	}
}

func TestHistoryCheckpointConcurrentRetentionKeepsOneImmutableReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	checkpoint := retainedHistoryCheckpoint()
	var group sync.WaitGroup
	references := make(chan string, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			store, err := reader.NewHistoryCheckpointStore(path)
			if err != nil {
				t.Error(err)
				return
			}
			reference, err := store.Retain(context.Background(), checkpoint)
			if err != nil {
				t.Error(err)
				return
			}
			references <- reference
		}()
	}
	group.Wait()
	close(references)
	expected := ""
	for reference := range references {
		if expected == "" {
			expected = reference
		}
		if expected != reference {
			t.Fatal("concurrent stores selected different checkpoints")
		}
	}
	if expected == "" {
		t.Fatal("no durable continuation published")
	}
}

func TestHistoryCheckpointFileLockHonorsRequestDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.db")
	store, err := reader.NewHistoryCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := store.Retain(context.Background(), retainedHistoryCheckpoint())
	if err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := store.Resolve(ctx, reference); err == nil || time.Since(started) > 500*time.Millisecond || ctx.Err() != nil && !errors.Is(err, ctx.Err()) || ctx.Err() == nil && kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("metadata lock exceeded request deadline: %v elapsed=%s", err, time.Since(started))
	}
}

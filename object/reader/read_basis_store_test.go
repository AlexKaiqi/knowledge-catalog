package reader_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func retainedBasis() objectmodel.ReadBasis {
	return objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders", Dataset: "orders", Revision: 1,
		Candidates: []objectmodel.ReadSource{{Repository: "kr://trade", Commit: "a1", Binding: "authority-a"}},
		Paths:      []objectmodel.ReadPath{{Repository: "kr://trade", Kind: "prefix"}}}
}

func TestReadBasisRecordsSurviveReopenAndRejectCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read-bases.db")
	store, err := reader.NewReadBasisStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	basis := retainedBasis()
	reference, err := store.Retain(ctx, basis)
	if err != nil {
		t.Fatal(err)
	}
	if same, err := store.Retain(ctx, basis); err != nil || same != reference {
		t.Fatalf("non-idempotent retention: %s %v", same, err)
	}
	store, err = reader.NewReadBasisStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Resolve(ctx, reference)
	if err != nil || kernel.CanonicalDigest(got) != kernel.CanonicalDigest(basis) {
		t.Fatalf("reopen lost original basis: %#v %v", got, err)
	}
	// Caller-owned slices must not mutate the retained record.
	got.Candidates[0].Commit = "newer"
	again, err := store.Resolve(ctx, reference)
	if err != nil || again.Candidates[0].Commit != "a1" {
		t.Fatalf("resolved record aliases mutable data: %#v %v", again, err)
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("object-read-bases-v1")).Put([]byte(reference), []byte(`{"version":1,"reference":"urn:kc:object:table/replaced"}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, reference); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corrupt record accepted: %v", err)
	}
	if _, err := store.Retain(ctx, basis); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corrupt record silently repaired: %v", err)
	}
}

func TestReadBasisRecordsMissingAndCanceledDoNotCreateAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	store, err := reader.NewReadBasisStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"", "../../outside", "rb1_bad"} {
		if _, err := store.Resolve(context.Background(), reference); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid reference %q: %v", reference, err)
		}
	}
	missing := "rb1_" + string(kernel.CanonicalDigest("not issued"))
	if _, err := store.Resolve(context.Background(), missing); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("missing basis substituted: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lookup created missing storage: %v", err)
	}
	invalid := retainedBasis()
	invalid.Candidates[0].Binding = ""
	if _, err := store.Retain(context.Background(), invalid); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("unbound source retained: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid basis created storage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Retain(ctx, retainedBasis()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled retention: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("canceled write created storage: %v", err)
	}
}

func TestConcurrentReadBasisRetentionPublishesOneImmutableRecord(t *testing.T) {
	store, err := reader.NewReadBasisStore(filepath.Join(t.TempDir(), "read-bases.db"))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	refs := make([]string, 8)
	errs := make([]error, len(refs))
	for i := range refs {
		workers.Add(1)
		go func() { defer workers.Done(); refs[i], errs[i] = store.Retain(context.Background(), retainedBasis()) }()
	}
	workers.Wait()
	for i, err := range errs {
		if err != nil || refs[i] == "" || refs[i] != refs[0] {
			t.Fatalf("concurrent retention %d: %s %v", i, refs[i], err)
		}
	}
	if _, err := store.Resolve(context.Background(), refs[0]); err != nil {
		t.Fatal(err)
	}
}

package control

import (
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
)

// A maintenance worker can outlive the journal it was opened with: the home
// that owns the file is removed at test exit, and bbolt then creates an empty
// database at the same path. Recording a failed head there must be a no-op, not
// a nil bucket dereference that crashes the process. finish already guards this
// case; failHead is on the same recovery path and needs the same guard.
func TestTargetStoreFailHeadToleratesAReinitializedJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	store := NewTargetStore(path)
	if err := store.ready(); err != nil {
		t.Fatal(err)
	}
	// Simulate the worker keeping its handle while the journal is replaced by an
	// empty database: every bucket the worker wrote to is gone.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := store.failHead("kr://acme/public/lost", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "authority unavailable")); err != nil {
		t.Fatalf("failHead against a reinitialized journal: %v", err)
	}
	targets, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("failHead invented a target row nobody asked for: %#v", targets)
	}
}

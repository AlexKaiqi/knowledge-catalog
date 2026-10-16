package writer_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/object/writer"
	"kc/snapshot"
)

// appliedPathsStore records the literal tree changes each commit sends.
type appliedPathsStore struct {
	snapshot.Store
	snapshot.TreeStore
	mu      sync.Mutex
	applied [][]snapshot.TreeChange
}

func (s *appliedPathsStore) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	s.mu.Lock()
	s.applied = append(s.applied, append([]snapshot.TreeChange(nil), cs.Changes...))
	s.mu.Unlock()
	return s.TreeStore.ApplyTreeCommit(cs)
}

func (s *appliedPathsStore) last() []snapshot.TreeChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applied[len(s.applied)-1]
}

func appliedPathsWriter(t *testing.T, id string) (*appliedPathsStore, *writer.Writer) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, id)
	s := &appliedPathsStore{Store: raw, TreeStore: raw.(snapshot.TreeStore)}
	registry := snapshot.NewRegistry()
	if err := registry.Add(s); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, w
}

// A value-only update keeps every locator and completion marker byte for byte;
// sending those files again only adds authority round trips.
func TestUpdateSkipsByteIdenticalFiles(t *testing.T) {
	s, w := appliedPathsWriter(t, "kr://writer/skip-unchanged")
	ledger := &basisReadLedger{Store: s.Store, TreeStore: s.TreeStore}
	root := testkit.MustHead(t, s, snapshot.DefaultRef)
	seed, err := w.Commit("skip-seed", basisReadOnceChanges(ledger, root, 16, 0, "seed"))
	if err != nil {
		t.Fatal(err)
	}
	seeded := len(s.last())
	update := basisReadOnceChanges(ledger, seed.Result.NewCommit, 16, 0, "updated")
	update.TargetRepository = s.ID()
	receipt, err := w.Commit("skip-update", update)
	if err != nil {
		t.Fatal(err)
	}
	changes := s.last()
	for _, change := range changes {
		if !strings.HasPrefix(change.Path, "notes/") {
			t.Fatalf("value-only update rewrote unchanged file %s (%d changes, seed sent %d)", change.Path, len(changes), seeded)
		}
	}
	if len(changes) != 16 {
		t.Fatalf("value-only update sent %d changes, want the 16 changed units", len(changes))
	}
	for _, index := range []int{0, 15} {
		raw, err := s.TreeStore.ReadFile(fmt.Sprintf("notes/%03d.json", index), receipt.Result.NewCommit)
		if err != nil || !strings.Contains(string(raw), "updated") {
			t.Fatalf("note %d not updated: %q err=%v", index, raw, err)
		}
	}
}

func timeWriterUpdate(t *testing.T, objects int) time.Duration {
	t.Helper()
	s, w := appliedPathsWriter(t, fmt.Sprintf("kr://writer/linear-%d", objects))
	ledger := &basisReadLedger{Store: s.Store, TreeStore: s.TreeStore}
	root := testkit.MustHead(t, s, snapshot.DefaultRef)
	seed, err := w.Commit(fmt.Sprintf("linear-seed-%d", objects), basisReadOnceChanges(ledger, root, objects, 0, "seed"))
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := w.Commit(fmt.Sprintf("linear-update-%d", objects), basisReadOnceChanges(ledger, seed.Result.NewCommit, objects, 0, "updated")); err != nil {
		t.Fatal(err)
	}
	return time.Since(started)
}

// Planning a full update used to be quadratic (whole-batch digest by string
// concatenation, sibling scans per operation and per object lookup).
func TestWriterUpdateScalesLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("scale guard runs in the full suite")
	}
	small, large := timeWriterUpdate(t, 500), timeWriterUpdate(t, 4000)
	if ratio := float64(large) / float64(small); ratio > 24 {
		t.Fatalf("Writer update grew %.1fx for 8x objects (%s -> %s)", ratio, small, large)
	}
}

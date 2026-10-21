package writer_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

// basisReadLedger records every authority read per fixed coordinate and the
// peak number of reads in flight. The authority is immutable per commit, so a
// repeated coordinate inside one commit is pure transport waste.
type basisReadLedger struct {
	snapshot.Store
	snapshot.TreeStore
	delay    time.Duration
	mu       sync.Mutex
	reads    map[string]int
	inFlight int
	peak     int
}

func (s *basisReadLedger) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	s.mu.Lock()
	s.reads[string(at)+"\x00"+path]++
	s.inFlight++
	if s.inFlight > s.peak {
		s.peak = s.inFlight
	}
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	raw, err := s.TreeStore.ReadFile(path, at)
	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return raw, err
}

func (s *basisReadLedger) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads, s.inFlight, s.peak = map[string]int{}, 0, 0
}

func (s *basisReadLedger) totals() (total, distinct, repeated int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, count := range s.reads {
		total += count
		distinct++
		if count > 1 {
			repeated++
		}
	}
	return total, distinct, repeated
}

func basisReadOnceWriter(t *testing.T) (*basisReadLedger, *writer.Writer, kernel.CommitID) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://writer/basis-read-once")
	s := &basisReadLedger{Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string]int{}}
	registry := snapshot.NewRegistry()
	if err := registry.Add(s); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, w, testkit.MustHead(t, raw, snapshot.DefaultRef)
}

func basisReadOnceChanges(s *basisReadLedger, base kernel.CommitID, objects, offset int, label string) objectmodel.ChangeSet {
	operations := []objectmodel.Operation{{
		Op:      objectmodel.OpPut,
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/read-once-note"},
		Value: map[string]any{"entity": "ReadOnceNote", "fields": map[string]any{
			"label": map[string]any{"type": "string", "required": true},
		}},
	}}
	for index := offset; index < offset+objects; index++ {
		operations = append(operations, objectmodel.Operation{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(fmt.Sprintf("note/%03d", index))},
			SchemaRef: "schema/read-once-note", PathHint: fmt.Sprintf("notes/%03d.json", index),
			Value: map[string]any{"label": fmt.Sprintf("%s %d", label, index)},
		})
	}
	return objectmodel.ChangeSet{
		TargetRepository: s.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Operations: operations,
	}
}

// A non-empty upload mixes updates of existing objects with new objects. Every
// basis coordinate must cross the authority at most once per commit: Schema
// planning, the tree codec and path admission share the same immutable view.
func TestNonemptyUploadReadsEachBasisFileOnce(t *testing.T) {
	s, w, root := basisReadOnceWriter(t)
	seed, err := w.Commit("read-once-seed", basisReadOnceChanges(s, root, 32, 0, "seed"))
	if err != nil {
		t.Fatal(err)
	}
	s.reset()
	cs := basisReadOnceChanges(s, seed.Result.NewCommit, 64, 0, "updated")
	receipt, err := w.Commit("read-once-update", cs)
	if err != nil {
		t.Fatal(err)
	}
	total, distinct, repeated := s.totals()
	if repeated != 0 || total != distinct {
		t.Fatalf("one commit re-read immutable basis files: total=%d distinct=%d repeated=%d", total, distinct, repeated)
	}
	if distinct == 0 {
		t.Fatal("non-empty upload skipped incremental old-unit checks")
	}
	for _, index := range []int{0, 31, 32, 63} {
		raw, err := s.TreeStore.ReadFile(fmt.Sprintf("notes/%03d.json", index), receipt.Result.NewCommit)
		if err != nil || len(raw) == 0 {
			t.Fatalf("upload did not publish note %d: %v", index, err)
		}
	}
}

// Old-unit reads are independent point lookups on an immutable commit; a
// large upload must not pay them as one serial round trip after another.
func TestNonemptyUploadReadsBasisConcurrently(t *testing.T) {
	s, w, root := basisReadOnceWriter(t)
	seed, err := w.Commit("concurrent-seed", basisReadOnceChanges(s, root, 64, 0, "seed"))
	if err != nil {
		t.Fatal(err)
	}
	s.reset()
	s.delay = 2 * time.Millisecond
	if _, err := w.Commit("concurrent-update", basisReadOnceChanges(s, seed.Result.NewCommit, 64, 0, "updated")); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	peak := s.peak
	s.mu.Unlock()
	if peak < 4 {
		t.Fatalf("basis point reads stayed serial: peak in-flight reads = %d", peak)
	}
}

// Shared planning reads keep the existing refusal semantics: a new identity
// may not take over an occupied path of an object outside the batch.
func TestReadOnceBasisKeepsPathOccupancyRefusal(t *testing.T) {
	s, w, root := basisReadOnceWriter(t)
	seed, err := w.Commit("occupancy-seed", basisReadOnceChanges(s, root, 4, 0, "seed"))
	if err != nil {
		t.Fatal(err)
	}
	cs := basisReadOnceChanges(s, seed.Result.NewCommit, 8, 4, "new")
	cs.Operations[1].PathHint = "notes/000.json"
	_, err = w.Commit("occupancy-steal", cs)
	testkit.ExpectCode(t, err, kernel.ErrObjectIDConflict)
	if testkit.MustHead(t, s, snapshot.DefaultRef) != seed.Result.NewCommit {
		t.Fatal("refused path takeover advanced the ref")
	}
}

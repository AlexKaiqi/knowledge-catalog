package writer_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

type noChangeReceiptFault struct {
	snapshot.CommandStore
	cas        snapshot.ConditionalCommandStore
	fail       atomic.Bool
	save       bool
	beforeSave func()
}

func (s *noChangeReceiptFault) CompareAndSwap(id string, expected, next *snapshot.CommandEntry) (bool, error) {
	if next != nil && next.Status == snapshot.StatusApplied && next.Phase == snapshot.CommandPhaseNoChange && s.fail.Swap(false) {
		if s.beforeSave != nil {
			s.beforeSave()
		}
		if s.save {
			if _, err := s.cas.CompareAndSwap(id, expected, next); err != nil {
				return false, err
			}
		}
		return false, errors.New("final receipt acknowledgement lost")
	}
	return s.cas.CompareAndSwap(id, expected, next)
}

func TestNoChangeWriterRecoveryRechecksOriginalBaseAndReplaysOnlyDurableSuccess(t *testing.T) {
	for _, row := range []struct {
		name           string
		saved, advance bool
	}{{"missing-receipt-same-head", false, false}, {"missing-receipt-head-advanced", false, true}, {"saved-receipt-head-advanced", true, true}} {
		t.Run(row.name, func(t *testing.T) {
			f := newNoChangeFixture(t, nil)
			seed := mustNoChangeCommit(t, f.writer, "seed", f.change(testkit.MustHead(t, f.repo, snapshot.DefaultRef), 1), true)
			cs := f.change(seed.Result.NewCommit, 1)
			store, err := snapshot.OpenBoltCommandStore(f.file)
			if err != nil {
				t.Fatal(err)
			}
			faulty := &noChangeReceiptFault{CommandStore: store, cas: store.(snapshot.ConditionalCommandStore), save: row.saved}
			faulty.fail.Store(true)
			var newest kernel.CommitID
			faulty.beforeSave = func() {
				if !row.advance {
					return
				}
				concurrent := objectmodel.ChangeSet{TargetRepository: f.repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: seed.Result.NewCommit, ExpectedTargetCommit: seed.Result.NewCommit, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/other"}, Value: 2}}}
				advanced := mustNoChangeCommit(t, f.writer, "concurrent-unrelated", concurrent, true)
				newest = advanced.Result.NewCommit
			}
			ledger, err := snapshot.NewCommandLedger(faulty)
			if err != nil {
				t.Fatal(err)
			}
			interrupted, err := writer.NewWriter(f.registry, ledger)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := interrupted.Commit("no-change", cs)
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || kernel.IsWriteNotApplied(err) || receipt.Result.NewCommit != "" {
				t.Fatalf("lost completion returned original success or non-application proof: %#v %v", receipt, err)
			}
			pending, ok := ledger.Lookup("no-change")
			if !ok || pending.Phase != snapshot.CommandPhaseNoChange {
				t.Fatalf("no durable no-commit constraint: %#v", pending)
			}
			coldStore, err := snapshot.OpenBoltCommandStore(f.file)
			if err != nil {
				t.Fatal(err)
			}
			cold, err := snapshot.NewCommandLedger(coldStore)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := writer.NewWriter(f.registry, cold)
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := reopened.CommitIntent("no-change", writer.CommitIntent{TargetRepository: cs.TargetRepository, TargetRef: cs.TargetRef, Operations: cs.Operations, Message: cs.Message, Provenance: cs.Provenance, BulkIngest: cs.BulkIngest})
			switch {
			case row.saved:
				if err != nil || resumed.Disposition != writer.DispositionReplayed || resumed.Result.Changed != objectmodel.ChangeUnchanged || resumed.Result.NewCommit != seed.Result.NewCommit {
					t.Fatalf("saved no-change result not replayed: %#v %v", resumed, err)
				}
			case row.advance:
				if kernel.CodeOf(err) != kernel.ErrNonFastForward || resumed.Result.NewCommit != "" {
					t.Fatalf("current same object body fabricated original success: %#v %v", resumed, err)
				}
			default:
				if err != nil || resumed.Disposition != writer.DispositionApplied || resumed.Result.Changed != objectmodel.ChangeUnchanged || resumed.Result.NewCommit != seed.Result.NewCommit {
					t.Fatalf("no-change was not freshly checked at original base: %#v %v", resumed, err)
				}
				completed, ok := cold.Lookup("no-change")
				if !ok || completed.Execution == pending.Execution {
					t.Fatalf("recovery reused old execution proof: %#v", completed)
				}
			}
			expected := seed.Result.NewCommit
			if row.advance {
				expected = newest
			}
			if testkit.MustHead(t, f.repo, snapshot.DefaultRef) != expected {
				t.Fatal("recovery advanced or rewound authoritative head")
			}
		})
	}
}

func TestNoChangeWriterReplayRejectsSameLogicalTargetReboundAtSameEndpoint(t *testing.T) {
	f := newNoChangeFixture(t, nil)
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(testkit.MustHead(t, f.repo, snapshot.DefaultRef), 1), true)
	cs := f.change(seed.Result.NewCommit, 1)
	mustNoChangeCommit(t, f.writer, "no-change", cs, false)
	replacement, err := snapshot.OpenLakeFS(f.repo.ID(), f.fake.DSN(f.fake.NewRepo()), f.fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	f.registry.Delete(f.repo.ID())
	if err := f.registry.Add(replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := f.writer.Commit("no-change", cs); kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
		t.Fatalf("receipt adopted a different physical authority: %v", err)
	}
}

func TestNoChangeWriterIntentDoesNotReplaceExplicitCoordinatesWithStoredBasis(t *testing.T) {
	f := newNoChangeFixture(t, nil)
	root := testkit.MustHead(t, f.repo, snapshot.DefaultRef)
	seed := mustNoChangeCommit(t, f.writer, "seed", f.change(root, 1), true)
	cs := f.change(seed.Result.NewCommit, 1)
	accepted := mustNoChangeCommit(t, f.writer, "no-change", cs, false)
	for _, row := range []struct{ base, expected kernel.CommitID }{{root, ""}, {"", root}, {root, root}} {
		_, err := f.writer.CommitIntent("no-change", writer.CommitIntent{TargetRepository: cs.TargetRepository, TargetRef: cs.TargetRef, BaseCommit: row.base, ExpectedTargetCommit: row.expected, Operations: cs.Operations})
		if kernel.CodeOf(err) != kernel.ErrIdempotencyConflict {
			t.Fatalf("explicit coordinates silently replaced: %#v %v", row, err)
		}
	}
	replay, err := f.writer.CommitIntent("no-change", writer.CommitIntent{TargetRepository: cs.TargetRepository, TargetRef: cs.TargetRef, Operations: cs.Operations})
	if err != nil || replay.Disposition != writer.DispositionReplayed || replay.Result != accepted.Result {
		t.Fatalf("omitted coordinates did not retain original basis: %#v %v", replay, err)
	}
}

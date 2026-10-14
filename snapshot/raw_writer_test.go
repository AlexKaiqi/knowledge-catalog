package snapshot_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// RawWrite goes through the same target/CAS/idempotency machinery as Commit,
// against a literal path instead of an Address.
func TestRawWriteAppliesAndAdvancesRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	receipt, err := s.TreeWriter.Commit("cmd-raw", snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/note.md", Content: []byte("draft\n")}},
		Message: "vfs write",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Result.NewCommit == s.RootCommitID {
		t.Fatal("commit did not advance")
	}
	raw, ok := snapshot.TreeStoreOf(s.Repo)
	if !ok {
		t.Fatal("fixture repository must implement RawFileStore")
	}
	got, err := raw.ReadFile("vfs/note.md", receipt.Result.NewCommit)
	if err != nil || string(got) != "draft\n" {
		t.Fatalf("%q %v", got, err)
	}
}

// Same command_id + same payload replays the stored receipt rather than
// writing again — the same idempotency contract Commit gives.
func TestRawWriteReplaysSameCommandID(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cs := snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/note.md", Content: []byte("draft\n")}},
	}
	first, err := s.TreeWriter.Commit("cmd-raw", cs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.TreeWriter.Commit("cmd-raw", cs)
	if err != nil {
		t.Fatal(err)
	}
	if second.Result.NewCommit != first.Result.NewCommit {
		t.Fatalf("replay must return the same commit: %v != %v", second.Result.NewCommit, first.Result.NewCommit)
	}
}

// Same command_id, different payload is a conflict, not a silent overwrite.
func TestRawWriteRejectsIdempotencyConflict(t *testing.T) {
	s := testkit.NewSetup(t, "")
	if _, err := s.TreeWriter.Commit("cmd-raw", snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/a.md", Content: []byte("a")}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.TreeWriter.Commit("cmd-raw", snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/b.md", Content: []byte("b")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrIdempotencyConflict)
}

func TestRawAndKnowledgeWritesShareCommandNamespace(t *testing.T) {
	s := testkit.NewSetup(t, "")
	if _, err := s.Writer.Commit("one-command", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{ObjectID: "note/a", Kind: objectmodel.KindEntity},
			Value:   map[string]any{"body": "knowledge"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.TreeWriter.Commit("one-command", snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "note.txt", Content: []byte("raw\n")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrIdempotencyConflict)
}

// A target with no TreeStore capability is refused at the seam, naming
// the missing capability — matching the pattern Store.Knowledge already
// established for the ② capability.
func TestRawWriteRefusesTargetWithoutCapability(t *testing.T) {
	s := testkit.NewSetup(t, "")
	plain := plainRawSnapshot{Store: s.Repo}
	if _, ok := snapshot.TreeStoreOf(plain); ok {
		t.Fatal("fixture must not implement RawFileStore")
	}
	store := snapshot.NewRegistry()
	if err := store.Add(plain); err != nil {
		t.Fatal(err)
	}
	w, err := snapshot.NewRawWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("cmd-raw", snapshot.TreeChangeSet{
		TargetRepository: plain.ID(), TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/a.md", Content: []byte("a")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
}

// plainRawSnapshot drops TreeStore the same way catalog's plainSnapshot
// drops Knowledge: embedding the interface, not the concrete type, so the
// method set is exactly what snapshot.Store declares.
type plainRawSnapshot struct{ snapshot.Store }

func TestRawWriteObserverCannotChangePublicationOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"nil", nil}, {"failure", errors.New("evidence unavailable")}} {
		t.Run("denied/"+tc.name, func(t *testing.T) {
			w, err := snapshot.NewRawWriter(snapshot.NewRegistry(), nil)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			w.SetObserver(func(surface string, _ map[string]any, outcome error) error {
				calls++
				if surface != snapshot.RawWriteSurface || kernel.CodeOf(outcome) != kernel.ErrTargetRepositoryDenied {
					t.Errorf("observer received surface = %s, outcome = %v", surface, outcome)
				}
				return tc.err
			})
			receipt, err := w.Commit("denied", snapshot.TreeChangeSet{
				TargetRepository: "kr://missing", TargetRef: snapshot.DefaultRef,
				Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("ignored")}},
			})
			testkit.ExpectCode(t, err, kernel.ErrTargetRepositoryDenied)
			if calls != 1 || receipt.Result.NewCommit != "" {
				t.Fatalf("denied result = %+v, observer calls = %d", receipt, calls)
			}
		})
	}

	t.Run("accepted", func(t *testing.T) {
		s := testkit.NewSetup(t, "")
		store := snapshot.NewBoltCommandStore(filepath.Join(t.TempDir(), "commands.db"))
		ledger, err := snapshot.NewCommandLedger(store)
		if err != nil {
			t.Fatal(err)
		}
		w, err := snapshot.NewRawWriter(s.Store, ledger)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		recordError := kernel.Fail(kernel.ErrTemporaryUnavailable, "evidence unavailable")
		observe := func(_ string, refs map[string]any, outcome error) error {
			calls++
			commit, ok := refs["newCommit"].(string)
			expectedDisposition := snapshot.ReceiptApplied
			if calls > 1 {
				expectedDisposition = snapshot.ReceiptReplayed
			}
			if outcome != nil || refs["disposition"] != string(expectedDisposition) || !ok || commit == "" {
				t.Errorf("observer did not receive accepted outcome: refs = %+v, error = %v", refs, outcome)
			}
			return recordError
		}
		w.SetObserver(observe)
		cs := snapshot.TreeChangeSet{
			TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
			BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
			Changes: []snapshot.TreeChange{{Path: "file.txt", Content: []byte("accepted")}},
		}
		receipt, err := w.Commit("accepted", cs)
		if !errors.Is(err, recordError) || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
			t.Errorf("accepted write swallowed its recording failure: %v", err)
		}
		if calls != 1 || receipt.Disposition != snapshot.ReceiptApplied || receipt.Result.NewCommit == "" || receipt.Result.NewCommit == s.RootCommitID {
			t.Fatalf("accepted result = %+v, error = %v, observer calls = %d", receipt, err, calls)
		}
		if testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != receipt.Result.NewCommit {
			t.Fatal("accepted receipt no longer matches published head")
		}
		restarted, err := snapshot.NewCommandLedger(store)
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := restarted.Lookup("accepted")
		var durable snapshot.Receipt
		if !ok || entry.Status != snapshot.StatusApplied || json.Unmarshal(entry.Receipt, &durable) != nil || durable != receipt {
			t.Fatalf("record failure changed accepted durable receipt: %+v, found = %v, receipt = %+v", entry, ok, durable)
		}
		w, err = snapshot.NewRawWriter(s.Store, restarted)
		if err != nil {
			t.Fatal(err)
		}
		w.SetObserver(observe)
		replay, err := w.Commit("accepted", cs)
		if !errors.Is(err, recordError) || kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
			t.Errorf("replay swallowed its recording failure: %v", err)
		}
		if calls != 2 || replay.Disposition != snapshot.ReceiptReplayed || replay.Result != receipt.Result || replay.ReceiptRef != receipt.ReceiptRef || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != receipt.Result.NewCommit {
			t.Fatalf("record failure caused another publication: replay = %+v, original = %+v, calls = %d", replay, receipt, calls)
		}
	})
}

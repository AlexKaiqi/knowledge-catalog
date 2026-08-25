package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/writer"
)

// RawWrite goes through the same target/CAS/idempotency machinery as Commit,
// against a literal path instead of an Address.
func TestRawWriteAppliesAndAdvancesRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	receipt, err := s.Writer.RawWrite("cmd-raw", snapshot.TreeChangeSet{
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
	first, err := s.Writer.RawWrite("cmd-raw", cs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Writer.RawWrite("cmd-raw", cs)
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
	if _, err := s.Writer.RawWrite("cmd-raw", snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/a.md", Content: []byte("a")}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Writer.RawWrite("cmd-raw", snapshot.TreeChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Changes: []snapshot.TreeChange{{Path: "vfs/b.md", Content: []byte("b")}},
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
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.RawWrite("cmd-raw", snapshot.TreeChangeSet{
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

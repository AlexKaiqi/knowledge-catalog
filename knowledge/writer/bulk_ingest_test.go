package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
	"kc/snapshot/commandlog"
)

// stagingOnlyStore hides the bulk capability of an otherwise capable store.
// Embedding the Store interface promotes only Store's methods; the TreeStore
// methods are re-declared explicitly, so BulkTreeIngesterOf must not discover
// this type. The ②→⓪ codec must fail closed on --bulk against it.
type stagingOnlyStore struct {
	snapshot.Store
	tree snapshot.TreeStore
}

func (s *stagingOnlyStore) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	return s.tree.ReadFile(path, commit)
}

func (s *stagingOnlyStore) ListFiles(commit kernel.CommitID) ([]string, error) {
	return s.tree.ListFiles(commit)
}

func (s *stagingOnlyStore) ApplyTreeCommit(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return s.tree.ApplyTreeCommit(cs)
}

func bulkPutOperation() knowledge.Operation {
	return knowledge.Operation{
		Op:      knowledge.OpPut,
		Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "dataset/1", AspectName: "structure"},
		Value:   map[string]any{"cols": 1},
	}
}

func bulkChangeSet(repoID kernel.RepositoryID, base kernel.CommitID, bulk bool) knowledge.CommitChangeSet {
	return knowledge.CommitChangeSet{
		TargetRepository:     repoID,
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           base,
		ExpectedTargetCommit: base,
		Operations:           []knowledge.Operation{bulkPutOperation()},
		BulkIngest:           bulk,
	}
}

func TestBulkIngestRejectsMediumWithoutCapability(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/bulkless")
	registry := snapshot.NewRegistry()
	if err := registry.Add(&stagingOnlyStore{Store: repo, tree: repo}); err != nil {
		t.Fatal(err)
	}
	commands, err := commandlog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, commands)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	_, err = w.Commit("bulk-reject", bulkChangeSet(repo.ID(), root, true))
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
	// The rejected command must not have applied anything.
	if got := testkit.MustHead(t, repo, snapshot.DefaultRef); got != root {
		t.Fatalf("rejected bulk commit moved the head: %s -> %s", root, got)
	}
}

func TestBulkIngestAppliesAndStaysReplayStable(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/bulk-apply")
	receipt, err := s.Writer.Commit("bulk-apply-1", bulkChangeSet(s.RepositoryID, s.RootCommitID, true))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != writer.DispositionApplied {
		t.Fatalf("first bulk commit disposition=%s", receipt.Disposition)
	}
	if receipt.Result.NewCommit == s.RootCommitID {
		t.Fatal("bulk commit did not advance the head")
	}
	replay, err := s.Writer.Commit("bulk-apply-1", bulkChangeSet(s.RepositoryID, s.RootCommitID, true))
	if err != nil {
		t.Fatal(err)
	}
	if replay.Disposition != writer.DispositionReplayed {
		t.Fatalf("identical bulk replay disposition=%s", replay.Disposition)
	}
	if replay.Result.NewCommit != receipt.Result.NewCommit {
		t.Fatalf("bulk replay changed commit: %s -> %s", receipt.Result.NewCommit, replay.Result.NewCommit)
	}
}

func TestBulkIngestModeFlipConflictsUnderOneCommandID(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/bulk-flip")
	if _, err := s.Writer.Commit("bulk-flip-1", bulkChangeSet(s.RepositoryID, s.RootCommitID, true)); err != nil {
		t.Fatal(err)
	}
	_, err := s.Writer.Commit("bulk-flip-1", bulkChangeSet(s.RepositoryID, s.RootCommitID, false))
	testkit.ExpectCode(t, err, kernel.ErrIdempotencyConflict)
}

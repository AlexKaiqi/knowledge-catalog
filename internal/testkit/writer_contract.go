package testkit

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

// WriterContract runs COMMIT idempotency, schema_ref, and branch/merge against any Repository.
//
// Args:
//
//	t: test handle.
//	create: builds an empty repository for the given id.
func WriterContract[R snapshot.Store](t *testing.T, create func(t *testing.T, id string) R) {
	t.Helper()

	t.Run("replays the same command_id and rejects a digest conflict", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/idem")
		root := MustHead(t, repo, "refs/heads/main")
		cs := objectmodel.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: PutEntity("fixture/a", 1, ""),
		}
		r1, err := w.Commit("cmd-1", cs)
		if err != nil {
			t.Fatal(err)
		}
		r2, err := w.Commit("cmd-1", cs)
		if err != nil {
			t.Fatal(err)
		}
		if r1.Disposition != writer.DispositionApplied || r2.Disposition != writer.DispositionReplayed {
			t.Fatalf("dispositions %s %s", r1.Disposition, r2.Disposition)
		}
		if r2.Result.CommitID != r1.Result.CommitID {
			t.Fatal("replay changed commit")
		}
		different := cs
		different.Operations = PutEntity("fixture/a", 2, "")
		_, err = w.Commit("cmd-1", different)
		ExpectCode(t, err, kernel.ErrIdempotencyConflict)
	})

	t.Run("accepts schema_ref in the same changeset and rejects a missing schema", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/schema")
		root := MustHead(t, repo, "refs/heads/main")
		_, err := w.Commit("bad-ref", objectmodel.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: []objectmodel.Operation{{
				Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"v": 1}, SchemaRef: "schema/policy",
			}},
		})
		ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
		if _, err := w.Commit("boot", objectmodel.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: map[string]any{"entity": "Policy", "pattern": "record"}},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"}, Value: map[string]any{"v": 1}, SchemaRef: "schema/policy"},
			},
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("branch commit leaves main still and merge publishes the reviewed commit", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/branch")
		main := MustHead(t, repo, "refs/heads/main")
		const branch = "refs/heads/review/PR-1"
		if _, err := w.CreateBranch(writer.BranchIntent{TargetRepository: repo.ID(), Ref: branch}); err != nil {
			t.Fatal(err)
		}
		receipt, err := w.CommitIntent("pr-1", writer.CommitIntent{
			TargetRepository: repo.ID(),
			TargetRef:        branch,
			Operations:       PutEntity("fixture/a", 1, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		if MustHead(t, repo, "refs/heads/main") != main {
			t.Fatal("branch commit moved main")
		}
		head, ok := repo.GetRef(branch)
		if !ok || head != receipt.Result.CommitID {
			t.Fatalf("branch %s %v", head, ok)
		}
		merged, err := w.Merge("merge-1", writer.MergeIntent{
			TargetRepository: repo.ID(), SourceRef: branch,
			SourceCommit: receipt.Result.CommitID, ExpectedTargetCommit: main,
		})
		if err != nil {
			t.Fatal(err)
		}
		if merged.Disposition != writer.DispositionApplied || MustHead(t, repo, "refs/heads/main") != receipt.Result.CommitID {
			t.Fatalf("merge %#v", merged)
		}
	})

}

func writerOn[R snapshot.Store](t *testing.T, create func(t *testing.T, id string) R, id string) (*writer.Writer, R) {
	t.Helper()
	repo := create(t, id)
	store := snapshot.NewRegistry()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return w, repo
}

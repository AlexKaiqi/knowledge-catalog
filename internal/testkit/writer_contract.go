package testkit

import (
	"testing"

	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

// WriterContract runs COMMIT idempotency, schema_ref, and PROPOSAL against any Repository.
//
// Args:
//
//	t: test handle.
//	create: builds an empty repository for the given id.
func WriterContract(t *testing.T, create func(t *testing.T, id string) repository.Repository) {
	t.Helper()

	t.Run("replays the same command_id and rejects a digest conflict", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/idem")
		root := MustHead(t, repo, "refs/heads/main")
		cs := repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: PutEntity("a", 1, ""),
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
		different.Operations = PutEntity("a", 2, "")
		_, err = w.Commit("cmd-1", different)
		ExpectCode(t, err, kernel.ErrIdempotencyConflict)
	})

	t.Run("accepts schema_ref in the same changeset and rejects a missing schema", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/schema")
		root := MustHead(t, repo, "refs/heads/main")
		_, err := w.Commit("bad-ref", repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: []repository.Operation{{
				Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"v": 1}, SchemaRef: "schema/policy",
			}},
		})
		ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
		if _, err := w.Commit("boot", repository.CommitChangeSet{
			TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
			BaseCommit: root, ExpectedTargetCommit: root,
			Operations: []repository.Operation{
				{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/policy"}, Value: map[string]any{"entity": "Policy", "pattern": "record"}},
				{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/A"}, Value: map[string]any{"v": 1}, SchemaRef: "schema/policy"},
			},
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("propose writes a candidate ref and leaves main still", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/propose")
		main := MustHead(t, repo, "refs/heads/main")
		receipt, err := w.Propose("pr-1", writer.ProposeIntent{
			TargetRepository: repo.ID(),
			TargetRef:        "refs/heads/main",
			CandidateRef:     "refs/heads/candidates/PR-1",
			BaseCommit:       main,
			Operations:       PutEntity("a", 1, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Surface != "PROPOSAL" {
			t.Fatal(receipt.Surface)
		}
		if MustHead(t, repo, "refs/heads/main") != main {
			t.Fatal("propose moved main")
		}
		cand, ok := repo.GetRef("refs/heads/candidates/PR-1")
		if !ok || cand != receipt.Result.CommitID {
			t.Fatalf("candidate %s %v", cand, ok)
		}
	})

	t.Run("rejects append after archive", func(t *testing.T) {
		w, repo := writerOn(t, create, "kr://conformance/writer/archive-append")
		if err := repo.Archive(); err != nil {
			t.Fatal(err)
		}
		_, err := w.Append("after-archive", repository.AppendEntries{
			TargetRepository: repo.ID(), StreamRef: "events",
			Entries: []repository.AppendEntry{{EventID: "e-1", Payload: 1}},
		})
		ExpectCode(t, err, kernel.ErrRepositoryArchived)
	})
}

func writerOn(t *testing.T, create func(t *testing.T, id string) repository.Repository, id string) (*writer.Writer, repository.Repository) {
	t.Helper()
	repo := create(t, id)
	store := repository.NewStore()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	BindJSONL(t, store, repo)
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return w, repo
}

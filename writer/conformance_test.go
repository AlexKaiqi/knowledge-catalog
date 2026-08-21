package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

func TestT1PathMove(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first := repository.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{{
			Op:       repository.OpPut,
			Address:  kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/P-103"},
			Value:    map[string]any{"statement": "production services require an owned runbook"},
			PathHint: "policies/P-103.yaml",
		}},
	}
	c1, err := s.Writer.Commit("cmd-1", first)
	if err != nil {
		t.Fatal(err)
	}
	move := repository.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           c1.Result.CommitID,
		ExpectedTargetCommit: c1.Result.CommitID,
		Operations: []repository.Operation{{
			Op:       repository.OpPut,
			Address:  kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/P-103"},
			Value:    map[string]any{"statement": "production services require an owned runbook"},
			PathHint: "policies/production/P-103.yaml",
		}},
	}
	c2, err := s.Writer.Commit("cmd-2", move)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Reader.Resolve(kernel.KnowledgeRef{Repository: s.RepositoryID, Object: "policy/P-103"}, c2.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != repository.StatusResolved || res.ObjectID != "policy/P-103" || res.PathHint != "policies/production/P-103.yaml" {
		t.Fatalf("unexpected resolution %#v", res)
	}
}

func TestT2CommitCAS(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first := repository.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations:           testkit.PutEntity("a", 1, ""),
	}
	if _, err := s.Writer.Commit("cmd-1", first); err != nil {
		t.Fatal(err)
	}
	stale := first
	stale.Operations = testkit.PutEntity("b", 2, "")
	_, err := s.Writer.Commit("cmd-2", stale)
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

func TestT3Atomicity(t *testing.T) {
	s := testkit.NewSetup(t, "")
	c1, err := s.Writer.Commit("cmd-1", repository.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations:           testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("cmd-2", repository.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           c1.Result.CommitID,
		ExpectedTargetCommit: c1.Result.CommitID,
		Operations: []repository.Operation{
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "b"}, Value: 2},
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "a"}, Value: 3, Precondition: &repository.Precondition{Type: repository.IfAbsent}},
		},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	head := testkit.MustHead(t, s.Repo, "")
	if head != c1.Result.CommitID {
		t.Fatalf("head moved to %s", head)
	}
	listed, err := s.Reader.List(s.RepositoryID, head)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Address.ObjectID != "a" {
		t.Fatalf("partial commit leaked: %#v", listed)
	}
}

func TestT4CommandIdempotency(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cs := repository.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations:           testkit.PutEntity("a", 1, ""),
	}
	r1, err := s.Writer.Commit("cmd-1", cs)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.Writer.Commit("cmd-1", cs)
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
	different.Operations = testkit.PutEntity("a", 2, "")
	_, err = s.Writer.Commit("cmd-1", different)
	testkit.ExpectCode(t, err, kernel.ErrIdempotencyConflict)
}

func TestT5AppendIdempotency(t *testing.T) {
	s := testkit.NewSetup(t, "")
	headBefore := testkit.MustHead(t, s.Repo, "refs/heads/main")
	entries := []repository.AppendEntry{{EventID: "evt-1", Payload: map[string]any{"outcome": "PASSED"}}}
	r1, err := s.Writer.Append("cmd-a", repository.AppendEntries{TargetRepository: s.RepositoryID, StreamRef: "evidence", Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Result.Appended) != 1 {
		t.Fatal(r1.Result.Appended)
	}
	r2, err := s.Writer.Append("cmd-b", repository.AppendEntries{TargetRepository: s.RepositoryID, StreamRef: "evidence", Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Result.Appended) != 1 || r2.Result.Appended[0] != r1.Result.Appended[0] {
		t.Fatalf("record ids %v %v", r1.Result.Appended, r2.Result.Appended)
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != headBefore {
		t.Fatal("append moved HEAD")
	}
	_, err = s.Writer.Append("cmd-c", repository.AppendEntries{
		TargetRepository: s.RepositoryID,
		StreamRef:        "evidence",
		Entries:          []repository.AppendEntry{{EventID: "evt-1", Payload: map[string]any{"outcome": "FAILED"}}},
	})
	testkit.ExpectCode(t, err, kernel.ErrEventIDConflict)
}

func TestT5ExpectedCursor(t *testing.T) {
	s := testkit.NewSetup(t, "")
	start := s.Stream.StreamCursor("ordered")
	if _, err := s.Writer.Append("cursor-1", repository.AppendEntries{
		TargetRepository: s.RepositoryID, StreamRef: "ordered", ExpectedCursor: start,
		Entries: []repository.AppendEntry{{EventID: "evt-1", Payload: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Writer.Append("cursor-stale", repository.AppendEntries{
		TargetRepository: s.RepositoryID, StreamRef: "ordered", ExpectedCursor: start,
		Entries: []repository.AppendEntry{{EventID: "evt-2", Payload: 2}},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	mid := s.Stream.StreamCursor("ordered")
	receipt, err := s.Writer.Append("cursor-2", repository.AppendEntries{
		TargetRepository: s.RepositoryID, StreamRef: "ordered", ExpectedCursor: mid,
		Entries: []repository.AppendEntry{{EventID: "evt-2", Payload: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	live := s.Stream.StreamCursor("ordered")
	if receipt.Result.Cursor != live || live == start || live == mid {
		t.Fatalf("cursor receipt=%s live=%s start=%s mid=%s", receipt.Result.Cursor, live, start, mid)
	}
}

func TestAppendIntentFillsCursor(t *testing.T) {
	s := testkit.NewSetup(t, "")
	start := s.Stream.StreamCursor("ordered")
	r1, err := s.Writer.AppendIntent("a1", writer.AppendIntent{
		TargetRepository: s.RepositoryID,
		StreamRef:        "ordered",
		Entries:          []repository.AppendEntry{{EventID: "e1", Payload: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Result.Cursor == start || r1.Result.Cursor != s.Stream.StreamCursor("ordered") {
		t.Fatalf("cursor %s start %s live %s", r1.Result.Cursor, start, s.Stream.StreamCursor("ordered"))
	}
	_, err = s.Writer.AppendIntent("stale", writer.AppendIntent{
		TargetRepository: s.RepositoryID,
		StreamRef:        "ordered",
		ExpectedCursor:   start,
		Entries:          []repository.AppendEntry{{EventID: "e2", Payload: 2}},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	r2, err := s.Writer.AppendIntent("a2", writer.AppendIntent{
		TargetRepository: s.RepositoryID,
		StreamRef:        "ordered",
		Entries:          []repository.AppendEntry{{EventID: "e2", Payload: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Result.Cursor == r1.Result.Cursor || r2.Result.Cursor != s.Stream.StreamCursor("ordered") {
		t.Fatalf("cursor %s prior %s live %s", r2.Result.Cursor, r1.Result.Cursor, s.Stream.StreamCursor("ordered"))
	}
	replay, err := s.Writer.AppendIntent("a1", writer.AppendIntent{
		TargetRepository: s.RepositoryID,
		StreamRef:        "ordered",
		Entries:          []repository.AppendEntry{{EventID: "e1", Payload: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Disposition != writer.DispositionReplayed || replay.Result.Cursor != r1.Result.Cursor {
		t.Fatalf("%#v", replay)
	}
}

func TestProposeSurface(t *testing.T) {
	s := testkit.NewSetup(t, "")
	main := testkit.MustHead(t, s.Repo, "refs/heads/main")
	receipt, err := s.Writer.Propose("pr-1", writer.ProposeIntent{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
		CandidateRef:     "refs/heads/candidates/PR-1",
		BaseCommit:       main,
		Operations:       testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Surface != "PROPOSAL" {
		t.Fatal(receipt.Surface)
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != main {
		t.Fatal("propose moved main")
	}
	cand, ok := s.Repo.GetRef("refs/heads/candidates/PR-1")
	if !ok || cand != receipt.Result.CommitID {
		t.Fatalf("candidate %s %v", cand, ok)
	}
	again, err := s.Writer.Propose("pr-1", writer.ProposeIntent{
		TargetRepository: s.RepositoryID,
		CandidateRef:     "refs/heads/candidates/PR-1",
		Operations:       testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Disposition != writer.DispositionReplayed || again.Result.CommitID != receipt.Result.CommitID {
		t.Fatalf("%#v", again)
	}
}

func TestEmptyChangeSetRejected(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("empty", repository.CommitChangeSet{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
	})
	testkit.ExpectCode(t, err, kernel.ErrWriteTargetRequired)
}

func TestT5ReadStream(t *testing.T) {
	s := testkit.NewSetup(t, "")
	headBefore := testkit.MustHead(t, s.Repo, "refs/heads/main")
	start := s.Stream.StreamCursor("runs")
	if _, err := s.Writer.Append("read-1", repository.AppendEntries{
		TargetRepository: s.RepositoryID, StreamRef: "runs",
		Entries: []repository.AppendEntry{{EventID: "run-1", Payload: map[string]any{"status": "ok"}}},
	}); err != nil {
		t.Fatal(err)
	}
	slice, err := s.Reader.ReadStream(s.RepositoryID, "runs")
	if err != nil {
		t.Fatal(err)
	}
	if slice.Cursor != s.Stream.StreamCursor("runs") || slice.Cursor == start || len(slice.Records) != 1 || slice.Records[0].EventID != "run-1" {
		t.Fatalf("%#v", slice)
	}
	payload, _ := slice.Records[0].Payload.(map[string]any)
	if payload["status"] != "ok" {
		t.Fatalf("payload %#v", slice.Records[0].Payload)
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != headBefore {
		t.Fatal("stream moved HEAD")
	}
	empty, err := s.Reader.ReadStream(s.RepositoryID, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Records) != 0 {
		t.Fatal(empty.Records)
	}
}

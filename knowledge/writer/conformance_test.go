package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/writer"
)

func TestT1PathMove(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first := knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:       knowledge.OpPut,
			Address:  knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/P-103"},
			Value:    map[string]any{"statement": "production services require an owned runbook"},
			PathHint: "policies/P-103.yaml",
		}},
	}
	c1, err := s.Writer.Commit("cmd-1", first)
	if err != nil {
		t.Fatal(err)
	}
	move := knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           c1.Result.CommitID,
		ExpectedTargetCommit: c1.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op:       knowledge.OpPut,
			Address:  knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/P-103"},
			Value:    map[string]any{"statement": "production services require an owned runbook"},
			PathHint: "policies/production/P-103.yaml",
		}},
	}
	c2, err := s.Writer.Commit("cmd-2", move)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Reader.Resolve(knowledge.KnowledgeRef{Repository: s.RepositoryID, Object: "policy/P-103"}, c2.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != knowledge.StatusResolved || res.ObjectID != "policy/P-103" || res.PathHint != "policies/production/P-103.yaml" {
		t.Fatalf("unexpected resolution %#v", res)
	}
}

func TestT2CommitCAS(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first := knowledge.CommitChangeSet{
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
	c1, err := s.Writer.Commit("cmd-1", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations:           testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("cmd-2", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           c1.Result.CommitID,
		ExpectedTargetCommit: c1.Result.CommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "b"}, Value: 2},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "a"}, Value: 3, Precondition: &knowledge.Precondition{Type: knowledge.IfAbsent}},
		},
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
	head := testkit.MustHead(t, s.Repo, "")
	if head != c1.Result.CommitID {
		t.Fatalf("head moved to %s", head)
	}
	repo, err := s.Reader.Require(s.RepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	scanner, err := knowledgemaintenance.RequireScanner(repo)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ScanSnapshotPage(head, knowledgemaintenance.ScanRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Values) != 1 || page.Values[0].Address.ObjectID != "a" {
		t.Fatalf("partial commit leaked: %#v", page.Values)
	}
}

func TestT4CommandIdempotency(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cs := knowledge.CommitChangeSet{
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
	_, err := s.Writer.Commit("empty", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
	})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

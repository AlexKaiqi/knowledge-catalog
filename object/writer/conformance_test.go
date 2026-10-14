package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	knowledgemaintenance "kc/object/maintenance"
	"kc/object/writer"
)

func TestT1PathMove(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first := objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:       objectmodel.OpPut,
			Address:  objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/P-103"},
			Value:    map[string]any{"statement": "production services require an owned runbook"},
			PathHint: "policies/P-103.yaml",
		}},
	}
	c1, err := s.Writer.Commit("cmd-1", first)
	if err != nil {
		t.Fatal(err)
	}
	move := objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           c1.Result.CommitID,
		ExpectedTargetCommit: c1.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op:       objectmodel.OpPut,
			Address:  objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/P-103"},
			Value:    map[string]any{"statement": "production services require an owned runbook"},
			PathHint: "policies/production/P-103.yaml",
		}},
	}
	c2, err := s.Writer.Commit("cmd-2", move)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Reader.Resolve(objectmodel.KnowledgeRef{Repository: s.RepositoryID, Object: "policy/P-103"}, c2.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != objectmodel.StatusResolved || res.ObjectID != "policy/P-103" || res.PathHint != "policies/production/P-103.yaml" {
		t.Fatalf("unexpected resolution %#v", res)
	}
}

func TestT2CommitCAS(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first := objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations:           testkit.PutEntity("a", 1, ""),
	}
	accepted, err := s.Writer.Commit("cmd-1", first)
	if err != nil {
		t.Fatal(err)
	}
	prior, ok := s.Writer.Lookup("cmd-1")
	if !ok {
		t.Fatal("accepted commit has no durable Writer receipt")
	}
	stale := first
	stale.Operations = testkit.PutEntity("b", 2, "")
	rejected, err := s.Writer.Commit("cmd-2", stale)
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
	if head := testkit.MustHead(t, s.Repo, "refs/heads/main"); head != accepted.Result.CommitID {
		t.Fatalf("rejected CAS advanced Ref: got %s, want %s", head, accepted.Result.CommitID)
	}
	if _, err := s.Reader.Read(objectmodel.KnowledgeRef{Repository: s.RepositoryID, Object: "b"}, accepted.Result.CommitID, nil); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("rejected CAS published b: %v", err)
	}
	original, err := s.Reader.Read(objectmodel.KnowledgeRef{Repository: s.RepositoryID, Object: "a"}, accepted.Result.CommitID, nil)
	if err != nil || kernel.CanonicalDigest(original.Value) != kernel.CanonicalDigest(1) {
		t.Fatalf("rejected CAS changed a: %#v, %v", original, err)
	}
	if rejected.Disposition != "" || rejected.Result.CommitID != "" || rejected.Result.NewCommit != "" {
		t.Fatalf("rejected CAS returned an accepted result: %#v", rejected)
	}
	if entry, ok := s.Writer.Lookup("cmd-2"); ok && entry.Receipt.Disposition == writer.DispositionApplied {
		t.Fatalf("rejected CAS has an APPLIED Writer result: %#v", entry)
	}
	if saved, ok := s.Writer.Lookup("cmd-1"); !ok || saved != prior {
		t.Fatalf("rejected CAS changed the accepted receipt: before=%#v after=%#v present=%v", prior, saved, ok)
	}
}

func TestT3Atomicity(t *testing.T) {
	s := testkit.NewSetup(t, "")
	c1, err := s.Writer.Commit("cmd-1", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations:           testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("cmd-2", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           c1.Result.CommitID,
		ExpectedTargetCommit: c1.Result.CommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "b"}, Value: 2},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "a"}, Value: 3, Precondition: &objectmodel.Precondition{Type: objectmodel.IfAbsent}},
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
	cs := objectmodel.CommitChangeSet{
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
	_, err := s.Writer.Commit("empty", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
	})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

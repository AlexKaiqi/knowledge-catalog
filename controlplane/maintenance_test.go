package controlplane_test

import (
	"testing"

	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/repository"
	"kc/writer"
)

type loop struct {
	testkit.Setup
	SupportRepo *local.FileGitRepository
	Catalog     *catalog.Catalog
	Definition  catalog.WorkspaceDefinition
	CP          *controlplane.ControlPlane
}

func setupLoop(t *testing.T) loop {
	t.Helper()
	base := testkit.NewSetup(t, "kr://acme/public/core")
	support := testkit.MakeRepository(t, "kr://acme/groups/support")
	if err := base.Store.Add(support); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, base.Store)
	def, err := cat.DefineWorkspace("maintenance", 1, []catalog.WorkspaceSource{
		{Repository: base.RepositoryID, Selector: "refs/heads/main"},
		{Repository: support.ID(), Selector: "refs/heads/main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return loop{
		Setup:       base,
		SupportRepo: support,
		Catalog:     cat,
		Definition:  def,
		CP:          controlplane.New(base.Store, base.Writer, cat),
	}
}

func commitToMain(t *testing.T, w *writer.Writer, repositoryID kernel.RepositoryID, base kernel.CommitID, objectID string, value any) kernel.CommitID {
	t.Helper()
	receipt, err := w.Commit("main:"+objectID+":"+string(base), repository.CommitChangeSet{
		TargetRepository: repositoryID, TargetRef: "refs/heads/main",
		BaseCommit: base, ExpectedTargetCommit: base,
		Operations: testkit.PutEntity(objectID, value, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt.Result.CommitID
}

func TestT9ProposalDoesNotMoveMain(t *testing.T) {
	s := setupLoop(t)
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	proposal, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-1", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-1",
		BaseCommit: base,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"v": "candidate"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.CandidateCommit == base {
		t.Fatal("candidate equals base")
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != base {
		t.Fatal("main moved")
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/candidates/PR-1") != proposal.CandidateCommit {
		t.Fatal("candidate ref mismatch")
	}
}

func TestT9ValidationBasisAndMovedCandidate(t *testing.T) {
	s := setupLoop(t)
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	p1, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-2", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-2",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview1, err := s.CP.CreatePreview("maintenance", p1)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview1.Repositories) != 2 {
		t.Fatal(preview1.Repositories)
	}
	live, err := s.Catalog.ResolveWorkspace("maintenance")
	if err != nil {
		t.Fatal(err)
	}
	if preview1.Repositories[s.SupportRepo.ID()] != live.Repositories[s.SupportRepo.ID()] {
		t.Fatal("support member changed")
	}
	val1, err := s.CP.RecordValidation(preview1, "S7", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	bad := val1
	bad.PreviewID = "other"
	_, err = s.CP.Merge(p1, preview1, bad)
	testkit.ExpectCode(t, err, kernel.ErrValidationBasisMismatch)

	p2, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-2b", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-2",
		BaseCommit: base, Operations: testkit.PutEntity("a", 2, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CP.Merge(p1, preview1, val1)
	testkit.ExpectCode(t, err, kernel.ErrCandidateMoved)

	preview2, err := s.CP.CreatePreview("maintenance", p2)
	if err != nil {
		t.Fatal(err)
	}
	val2, err := s.CP.RecordValidation(preview2, "S7", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := s.CP.Merge(p2, preview2, val2)
	if err != nil || merged != p2.CandidateCommit {
		t.Fatal(merged, err)
	}
}

func TestT9ValidateStructure(t *testing.T) {
	s := setupLoop(t)
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	proposal, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-struct", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-struct",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", proposal)
	if err != nil {
		t.Fatal(err)
	}
	passed, err := s.CP.ValidateStructure(preview)
	if err != nil || passed.Outcome != "PASSED" || passed.SuiteRevision != "structure" || len(passed.Check.Issues) != 0 {
		t.Fatalf("%#v %v", passed, err)
	}
	s.Store.Delete(s.RepositoryID)
	failed, err := s.CP.ValidateStructure(preview)
	if err != nil || failed.Outcome != "FAILED" {
		t.Fatal(failed, err)
	}
	found := false
	for _, issue := range failed.Check.Issues {
		if issue.Code == kernel.ErrUsageInvalid {
			found = true
		}
	}
	if !found {
		t.Fatal(failed.Check.Issues)
	}
}

func TestT9MergeDoesNotNeedPromote(t *testing.T) {
	s := setupLoop(t)
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	p, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-3", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-3",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", p)
	if err != nil {
		t.Fatal(err)
	}
	val, err := s.CP.RecordValidation(preview, "S7", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := s.CP.Merge(p, preview, val)
	if err != nil {
		t.Fatal(err)
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != merged {
		t.Fatal("main not merged")
	}
	got, err := testkit.FederatedRead(s.Catalog, "maintenance", "a")
	if err != nil || len(got) == 0 {
		t.Fatal(got, err)
	}
}

func TestT9MergeRejectsMovedMain(t *testing.T) {
	s := setupLoop(t)
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	p, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-4", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-4",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", p)
	if err != nil {
		t.Fatal(err)
	}
	val, err := s.CP.RecordValidation(preview, "S7", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	if commitToMain(t, s.Writer, s.RepositoryID, base, "other", 99) == base {
		t.Fatal("main did not move")
	}
	_, err = s.CP.Merge(p, preview, val)
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

func TestMergeGateRequiresSuite(t *testing.T) {
	s := setupLoop(t)
	var stored []controlplane.ValidationReport
	s.CP.SetMergeGate(func(repo kernel.RepositoryID) []string {
		if repo != s.RepositoryID {
			return nil
		}
		return []string{"suite:metrics-contract"}
	}, func(basis string) []gate.Evidence {
		out := []gate.Evidence{}
		for _, report := range stored {
			if report.PreviewID == basis {
				out = append(out, gate.Evidence{Name: report.SuiteRevision, BasisID: basis, Outcome: report.Outcome})
			}
		}
		return out
	})
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	p, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-gate", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-gate",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", p)
	if err != nil {
		t.Fatal(err)
	}
	structure, err := s.CP.ValidateStructure(preview)
	if err != nil {
		t.Fatal(err)
	}
	stored = append(stored, structure.ValidationReport)
	_, err = s.CP.Merge(p, preview, controlplane.ValidationReport{})
	testkit.ExpectCode(t, err, kernel.ErrGateUnsatisfied)

	suite, err := s.CP.RecordValidation(preview, "metrics-contract", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	stored = append(stored, suite)
	if _, err := s.CP.Merge(p, preview, controlplane.ValidationReport{}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeGateValidateStructureSatisfiesValidate(t *testing.T) {
	s := setupLoop(t)
	var stored []controlplane.ValidationReport
	s.CP.SetMergeGate(func(repo kernel.RepositoryID) []string {
		return []string{"validate"}
	}, func(basis string) []gate.Evidence {
		out := []gate.Evidence{}
		for _, report := range stored {
			if report.PreviewID == basis {
				out = append(out, gate.Evidence{Name: report.SuiteRevision, BasisID: basis, Outcome: report.Outcome})
			}
		}
		return out
	})
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	p, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-val", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-val",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CP.Merge(p, preview, controlplane.ValidationReport{})
	testkit.ExpectCode(t, err, kernel.ErrGateUnsatisfied)
	structure, err := s.CP.ValidateStructure(preview)
	if err != nil {
		t.Fatal(err)
	}
	stored = append(stored, structure.ValidationReport)
	if _, err := s.CP.Merge(p, preview, controlplane.ValidationReport{}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeGateFailedOutcome(t *testing.T) {
	s := setupLoop(t)
	var stored []controlplane.ValidationReport
	s.CP.SetMergeGate(func(repo kernel.RepositoryID) []string {
		return []string{"suite:lint"}
	}, func(basis string) []gate.Evidence {
		out := []gate.Evidence{}
		for _, report := range stored {
			if report.PreviewID == basis {
				out = append(out, gate.Evidence{Name: report.SuiteRevision, BasisID: basis, Outcome: report.Outcome})
			}
		}
		return out
	})
	base := testkit.MustHead(t, s.Repo, "refs/heads/main")
	p, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "PR-fail", RepositoryID: s.RepositoryID,
		TargetRef: "refs/heads/main", CandidateRef: "refs/heads/candidates/PR-fail",
		BaseCommit: base, Operations: testkit.PutEntity("a", 1, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", p)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := s.CP.RecordValidation(preview, "lint", "FAILED")
	if err != nil {
		t.Fatal(err)
	}
	stored = []controlplane.ValidationReport{failed}
	_, err = s.CP.Merge(p, preview, failed)
	testkit.ExpectCode(t, err, kernel.ErrGateUnsatisfied)
	passed, err := s.CP.RecordValidation(preview, "lint", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	stored = []controlplane.ValidationReport{passed}
	if _, err := s.CP.Merge(p, preview, passed); err != nil {
		t.Fatal(err)
	}
}

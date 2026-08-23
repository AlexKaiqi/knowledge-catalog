package scenario

import (
	"testing"

	"kc/catalog"
	"kc/controlplane"
	"kc/gate"
	"kc/kernel"
	"kc/repository"
)

func s2DefineWorkspace(t *testing.T, wb *workbench) {
	wb.stamp("steward", "s2-define", "")
	if _, err := wb.catalog.DefineWorkspace(ViewBoard, 1, companyWorkspaceSources()); err != nil {
		t.Fatal(err)
	}
	wb.expectCatalog(t, catalogWant{
		workspaces: []workspaceWant{{id: ViewBoard, rev: 1, repos: []kernel.RepositoryID{Metadata, Semantics}}},
	})

	table, err := wb.federatedRead(ViewBoard, TableTrade)
	if err != nil || len(table) != 1 || table[0].Repository != Metadata {
		t.Fatalf("%#v %v", table, err)
	}
	gmv, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || len(gmv) != 0 {
		t.Fatalf("claimed GMV must be absent before merge: %#v %v", gmv, err)
	}
	serving, err := wb.openView(ViewBoard)
	if err != nil {
		t.Fatal(err)
	}
	example, err := serving.Resolve(ExampleGMV)
	if err != nil || len(example) != 1 || example[0].Status != repository.StatusResolved {
		t.Fatalf("%#v %v", example, err)
	}
	hist := wb.catalog.Log(catalog.CatalogLogQuery{Workspace: ViewBoard, Limit: 10})
	if len(hist.Commits) == 0 {
		t.Fatal("define-workspace must appear in catalog log")
	}
}

func s3ClaimGMV(t *testing.T, wb *workbench) {
	wb.stamp("steward", "s3-propose", "rule-gmv")
	before := wb.freeze(t)
	proposal, err := wb.plane.Propose(controlplane.ProposeInput{
		ProposalID:   "PR-gmv",
		RepositoryID: Semantics,
		TargetRef:    MainRef,
		CandidateRef: "refs/heads/candidates/PR-gmv",
		BaseCommit:   wb.commits["S1"],
		Operations:   gmvClaim(GMVCompany),
		Rationale:    "认领 GMV：不含 7 日内退货",
		Provenance:   definitionEnvelope("steward"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if wb.head(t, Semantics) != wb.commits["S1"] {
		t.Fatal("propose moved main")
	}
	wb.expectUnchanged(t, before)

	preview, err := wb.plane.CreatePreview(ViewBoard, proposal)
	if err != nil {
		t.Fatal(err)
	}
	wb.rememberCommit("Cpv", proposal.CandidateCommit)
	wb.expectUnchanged(t, before)
	previewGMV := readPreview(t, wb, preview, MetricGMV)
	if len(previewGMV) == 0 || nestedString(previewGMV[0].Value, "definition", "formula") != GMVCompany {
		t.Fatalf("preview %#v", previewGMV)
	}
	stableGMV, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || len(stableGMV) != 0 {
		t.Fatalf("preview must not move the published branch: %#v", stableGMV)
	}

	structure, err := wb.plane.ValidateStructure(preview)
	if err != nil || structure.Outcome != "PASSED" {
		t.Fatal(structure, err)
	}
	frozen := wb.freeze(t)
	_, err = wb.plane.Merge(proposal, preview, structure.ValidationReport)
	expectCode(t, err, kernel.ErrGateUnsatisfied)
	wb.expectUnchanged(t, frozen)
	if wb.head(t, Semantics) != wb.commits["S1"] {
		t.Fatal("failed merge moved main")
	}

	pid := preview.PreviewID
	wb.evidence[pid] = []gate.Evidence{{
		Name: gate.StructureSuite, BasisID: pid, Outcome: "PASSED",
	}}
	steward, err := wb.plane.RecordValidation(preview, "steward", "PASSED")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := wb.plane.Merge(proposal, preview, steward)
	if err != nil {
		t.Fatal(err)
	}
	wb.rememberCommit("C1", merged)
	if wb.head(t, Semantics) != merged {
		t.Fatal("merge did not fast-forward main")
	}
	wb.expectUnchanged(t, frozen)

	live, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || len(live) != 1 || nestedString(live[0].Value, "definition", "formula") != GMVCompany {
		t.Fatalf("merge must be visible on next OpenWorkspace: %#v %v", live, err)
	}

	s3CandidateMoved(t, wb)
}

func s3CandidateMoved(t *testing.T, wb *workbench) {
	t.Helper()
	p1, err := wb.plane.Propose(controlplane.ProposeInput{
		ProposalID:   "PR-scratch",
		RepositoryID: Semantics,
		TargetRef:    MainRef,
		CandidateRef: "refs/heads/candidates/PR-scratch",
		BaseCommit:   wb.commits["C1"],
		Operations:   []repository.Operation{putEntity(DraftScratch, map[string]any{"v": "one"}, "")},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := wb.plane.CreatePreview(ViewBoard, p1)
	if err != nil {
		t.Fatal(err)
	}
	wb.rememberCommit("Scratch1", p1.CandidateCommit)
	structure, err := wb.plane.ValidateStructure(preview)
	if err != nil {
		t.Fatal(err)
	}
	pid := preview.PreviewID
	wb.evidence[pid] = []gate.Evidence{
		{Name: gate.StructureSuite, BasisID: pid, Outcome: "PASSED"},
		{Name: "steward", BasisID: pid, Outcome: "PASSED"},
	}
	_, err = wb.plane.Propose(controlplane.ProposeInput{
		ProposalID:   "PR-scratch-b",
		RepositoryID: Semantics,
		TargetRef:    MainRef,
		CandidateRef: "refs/heads/candidates/PR-scratch",
		BaseCommit:   wb.commits["C1"],
		Operations:   []repository.Operation{putEntity(DraftScratch, map[string]any{"v": "two"}, "")},
	})
	if err != nil {
		t.Fatal(err)
	}
	frozen := wb.freeze(t)
	_, err = wb.plane.Merge(p1, preview, structure.ValidationReport)
	expectCode(t, err, kernel.ErrCandidateMoved)
	wb.expectUnchanged(t, frozen)
	if wb.head(t, Semantics) != wb.commits["C1"] {
		t.Fatal("moved candidate merge advanced main")
	}
}

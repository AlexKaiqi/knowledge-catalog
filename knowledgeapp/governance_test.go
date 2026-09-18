package knowledgeapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"kc/catalog"
	"kc/controlplane"
	"kc/kernel"
)

type governanceRecorder struct {
	proposeInput controlplane.ProposeInput
	workspace    catalog.ResolvedKnowledgeSet
	proposal     controlplane.Proposal
	preview      controlplane.Preview
	validation   controlplane.ValidationReport
	suite        string
	outcome      string
	observed     bool
}

func (r *governanceRecorder) Propose(input controlplane.ProposeInput) (controlplane.Proposal, error) {
	r.proposeInput = input
	return controlplane.Proposal{ProposalID: input.ProposalID}, nil
}

func (r *governanceRecorder) CreatePreviewAt(workspace catalog.ResolvedKnowledgeSet, proposal controlplane.Proposal) (controlplane.Preview, error) {
	r.workspace, r.proposal = workspace, proposal
	return controlplane.Preview{PreviewID: "preview-1"}, nil
}

func (r *governanceRecorder) ValidateStructure(preview controlplane.Preview) (controlplane.StructureReport, error) {
	r.preview = preview
	return controlplane.StructureReport{
		ValidationReport: controlplane.ValidationReport{ReportID: "structure-1"},
	}, nil
}

func (r *governanceRecorder) RecordValidation(preview controlplane.Preview, suite, outcome string) (controlplane.ValidationReport, error) {
	r.preview, r.suite, r.outcome = preview, suite, outcome
	return controlplane.ValidationReport{ReportID: "validation-1"}, nil
}

func (r *governanceRecorder) MergeObserved(
	proposal controlplane.Proposal,
	preview controlplane.Preview,
	validation controlplane.ValidationReport,
	observe controlplane.MergeGateObserver,
) (kernel.CommitID, error) {
	r.proposal, r.preview, r.validation = proposal, preview, validation
	observe(2, "ok", time.Millisecond)
	r.observed = true
	return kernel.CommitID("commit-1"), nil
}

func TestGovernanceExecutorsPreserveTypedRequestsAndPersistence(t *testing.T) {
	ctx := context.Background()
	recorder := &governanceRecorder{}

	proposal, err := (ProposalExecutor{
		Control: recorder,
		Save: func(proposal controlplane.Proposal) error {
			if proposal.ProposalID != "proposal-1" {
				t.Fatalf("unexpected proposal save: %#v", proposal)
			}
			return nil
		},
	}).Execute(ctx, ProposalRequest{Input: controlplane.ProposeInput{ProposalID: "proposal-1"}})
	if err != nil || proposal.ProposalID != "proposal-1" || recorder.proposeInput.ProposalID != "proposal-1" {
		t.Fatalf("proposal request was not preserved: %#v %v", proposal, err)
	}

	workspace := catalog.ResolvedKnowledgeSet{SetID: "workspace-1", PinID: "pin-1"}
	preview, err := (PreviewExecutor{
		Control: recorder,
		Save: func(preview controlplane.Preview) error {
			if preview.PreviewID != "preview-1" {
				t.Fatalf("unexpected preview save: %#v", preview)
			}
			return nil
		},
	}).Execute(ctx, PreviewRequest{Resolved: workspace, Proposal: proposal})
	if err != nil || recorder.workspace.PinID != workspace.PinID || recorder.proposal.ProposalID != proposal.ProposalID {
		t.Fatalf("preview request was not preserved: %#v %v", preview, err)
	}

	structure, err := (ValidatePreviewExecutor{
		Control: recorder,
		Save: func(report controlplane.ValidationReport) error {
			if report.ReportID != "structure-1" {
				t.Fatalf("unexpected structure save: %#v", report)
			}
			return nil
		},
	}).Execute(ctx, ValidatePreviewRequest{Preview: preview})
	if err != nil || structure.ReportID != "structure-1" || recorder.preview.PreviewID != preview.PreviewID {
		t.Fatalf("validate request was not preserved: %#v %v", structure, err)
	}

	validation, err := (RecordValidationExecutor{
		Control: recorder,
		Save: func(report controlplane.ValidationReport) error {
			if report.ReportID != "validation-1" {
				t.Fatalf("unexpected validation save: %#v", report)
			}
			return nil
		},
	}).Execute(ctx, RecordValidationRequest{
		Preview: preview, SuiteRevision: "suite-1", Outcome: "PASSED",
	})
	if err != nil || recorder.suite != "suite-1" || recorder.outcome != "PASSED" {
		t.Fatalf("record request was not preserved: %#v %v", validation, err)
	}

	result, err := (MergeProposalExecutor{Control: recorder}).Execute(ctx, MergeProposalRequest{
		Proposal:       proposal,
		Preview:        preview,
		Validation:     validation,
		RequiredChecks: []string{"suite-1"},
		ObserveGate:    func(int, string, time.Duration) {},
	})
	if err != nil || result.CommitID != "commit-1" || result.ProposalID != proposal.ProposalID ||
		result.PreviewID != preview.PreviewID || result.Gate.Basis != preview.PreviewID ||
		len(result.Gate.Required) != 1 || !recorder.observed {
		t.Fatalf("merge request/result was not preserved: %#v %v", result, err)
	}
}

func TestGovernanceExecutorReturnsPersistenceFailure(t *testing.T) {
	persistErr := errors.New("persist control")
	proposal, err := (ProposalExecutor{
		Control: &governanceRecorder{},
		Save:    func(controlplane.Proposal) error { return persistErr },
	}).Execute(context.Background(), ProposalRequest{
		Input: controlplane.ProposeInput{ProposalID: "proposal-1"},
	})
	if !errors.Is(err, persistErr) || proposal != (controlplane.Proposal{}) {
		t.Fatalf("got proposal %#v and error %v", proposal, err)
	}
}

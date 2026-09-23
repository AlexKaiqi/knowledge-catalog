package knowledgeapp

import (
	"context"

	"kc/catalog"
	"kc/controlplane"
	"kc/kernel"
)

type ProposalCreator interface {
	Propose(controlplane.ProposeInput) (controlplane.Proposal, error)
}

type ProposalRequest struct {
	Input controlplane.ProposeInput
}

type ProposalExecutor struct {
	Control ProposalCreator
	Save    func(controlplane.Proposal) error
}

func (e ProposalExecutor) Execute(_ context.Context, request ProposalRequest) (controlplane.Proposal, error) {
	if e.Control == nil || e.Save == nil {
		return controlplane.Proposal{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"governance proposal service is unavailable")
	}
	proposal, err := e.Control.Propose(request.Input)
	if err != nil {
		return controlplane.Proposal{}, err
	}
	if err := e.Save(proposal); err != nil {
		return controlplane.Proposal{}, err
	}
	return proposal, nil
}

type PreviewCreator interface {
	CreatePreviewAt(catalog.ResolvedKnowledgeSet, controlplane.Proposal) (controlplane.Preview, error)
}

type PreviewRequest struct {
	Resolved catalog.ResolvedKnowledgeSet
	Proposal controlplane.Proposal
}

type PreviewExecutor struct {
	Control PreviewCreator
	Save    func(controlplane.Preview) error
}

func (e PreviewExecutor) Execute(_ context.Context, request PreviewRequest) (controlplane.Preview, error) {
	if e.Control == nil || e.Save == nil {
		return controlplane.Preview{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"governance preview service is unavailable")
	}
	preview, err := e.Control.CreatePreviewAt(request.Resolved, request.Proposal)
	if err != nil {
		return controlplane.Preview{}, err
	}
	if err := e.Save(preview); err != nil {
		return controlplane.Preview{}, err
	}
	return preview, nil
}

type StructureValidator interface {
	ValidateStructure(controlplane.Preview) (controlplane.StructureReport, error)
}

type ValidatePreviewRequest struct {
	Preview controlplane.Preview
}

type ValidatePreviewExecutor struct {
	Control StructureValidator
	Save    func(controlplane.ValidationReport) error
}

func (e ValidatePreviewExecutor) Execute(_ context.Context, request ValidatePreviewRequest) (controlplane.StructureReport, error) {
	if e.Control == nil || e.Save == nil {
		return controlplane.StructureReport{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"governance validation service is unavailable")
	}
	report, err := e.Control.ValidateStructure(request.Preview)
	if err != nil {
		return controlplane.StructureReport{}, err
	}
	if err := e.Save(report.ValidationReport); err != nil {
		return controlplane.StructureReport{}, err
	}
	return report, nil
}

type ValidationRecorder interface {
	RecordValidation(controlplane.Preview, string, string) (controlplane.ValidationReport, error)
}

type RecordValidationRequest struct {
	Preview       controlplane.Preview
	SuiteRevision string
	Outcome       string
}

type RecordValidationExecutor struct {
	Control ValidationRecorder
	Save    func(controlplane.ValidationReport) error
}

func (e RecordValidationExecutor) Execute(_ context.Context, request RecordValidationRequest) (controlplane.ValidationReport, error) {
	if e.Control == nil || e.Save == nil {
		return controlplane.ValidationReport{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"governance validation service is unavailable")
	}
	report, err := e.Control.RecordValidation(request.Preview, request.SuiteRevision, request.Outcome)
	if err != nil {
		return controlplane.ValidationReport{}, err
	}
	if err := e.Save(report); err != nil {
		return controlplane.ValidationReport{}, err
	}
	return report, nil
}

type ProposalMerger interface {
	MergeObserved(controlplane.Proposal, controlplane.Preview, controlplane.ValidationReport, controlplane.MergeGateObserver) (kernel.CommitID, error)
}

type MergeProposalRequest struct {
	Proposal       controlplane.Proposal
	Preview        controlplane.Preview
	Validation     controlplane.ValidationReport
	RequiredChecks []string
	ObserveGate    controlplane.MergeGateObserver
}

type MergeGateResult struct {
	Status   string   `json:"status"`
	Basis    string   `json:"basis"`
	Required []string `json:"required"`
}

type MergeProposalResult struct {
	CommitID   kernel.CommitID     `json:"commitId"`
	ProposalID string              `json:"proposalId"`
	PreviewID  string              `json:"previewId"`
	Repository kernel.RepositoryID `json:"repository"`
	TargetRef  string              `json:"targetRef"`
	Gate       MergeGateResult     `json:"gate"`
}

type MergeProposalExecutor struct {
	Control ProposalMerger
}

func (e MergeProposalExecutor) Execute(_ context.Context, request MergeProposalRequest) (MergeProposalResult, error) {
	if e.Control == nil {
		return MergeProposalResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"governance merge service is unavailable")
	}
	commitID, err := e.Control.MergeObserved(
		request.Proposal, request.Preview, request.Validation, request.ObserveGate,
	)
	if err != nil {
		return MergeProposalResult{}, err
	}
	return MergeProposalResult{
		CommitID:   commitID,
		ProposalID: request.Proposal.ProposalID,
		PreviewID:  request.Preview.PreviewID,
		Repository: request.Proposal.TargetRepository,
		TargetRef:  request.Proposal.TargetRef,
		Gate: MergeGateResult{
			Status:   "PASSED",
			Basis:    request.Preview.PreviewID,
			Required: request.RequiredChecks,
		},
	}, nil
}

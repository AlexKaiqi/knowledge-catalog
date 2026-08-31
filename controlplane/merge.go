package controlplane

import (
	"time"

	"kc/gate"
	"kc/kernel"
	"kc/snapshot"
)

type MergeGateObserver func(required int, outcome string, elapsed time.Duration)

func (cp *ControlPlane) Merge(proposal Proposal, preview Preview, validation ValidationReport) (commitID kernel.CommitID, err error) {
	return cp.merge(proposal, preview, validation, nil)
}

func (cp *ControlPlane) MergeObserved(proposal Proposal, preview Preview, validation ValidationReport, observe MergeGateObserver) (commitID kernel.CommitID, err error) {
	return cp.merge(proposal, preview, validation, observe)
}

func (cp *ControlPlane) merge(proposal Proposal, preview Preview, validation ValidationReport, observe MergeGateObserver) (commitID kernel.CommitID, err error) {
	refs := map[string]any{"proposalId": proposal.ProposalID, "previewId": preview.PreviewID, "reportId": validation.ReportID}
	defer func() { err = cp.note("merge", refs, err) }()
	repo, err := cp.store.Require(proposal.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return "", err
	}
	if preview.Candidate.RepositoryID != proposal.TargetRepository ||
		preview.Candidate.CommitID != proposal.CandidateCommit ||
		preview.Repositories[proposal.TargetRepository] != proposal.CandidateCommit ||
		preview.BaseCommit != proposal.BaseCommit {
		return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
	}
	if validation.ReportID != "" && validation.PreviewID != preview.PreviewID {
		return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
	}
	gateStarted := time.Now()
	required, gateErr := cp.checkMergeGate(proposal, preview, validation)
	if observe != nil {
		outcome := "ok"
		if gateErr != nil {
			outcome = "error"
		}
		observe(len(required), outcome, time.Since(gateStarted))
	}
	if gateErr != nil {
		return "", gateErr
	}
	if current, ok := repo.GetRef(proposal.CandidateRef); !ok || current != proposal.CandidateCommit {
		return "", kernel.Fail(kernel.ErrCandidateMoved, "candidate advanced after validation")
	}
	commitID, err = repo.Merge(proposal.TargetRef, proposal.CandidateCommit, proposal.BaseCommit)
	if err != nil {
		return "", err
	}
	cp.store.NotifyAdvanced(snapshot.Advanced{
		Store: repo,
		From:  proposal.BaseCommit,
		To:    commitID,
	})
	refs["commitId"] = string(commitID)
	return commitID, nil
}

func (cp *ControlPlane) checkMergeGate(proposal Proposal, preview Preview, validation ValidationReport) ([]string, error) {
	required := []string{}
	if cp.mergeRequired != nil {
		required = cp.mergeRequired(proposal.TargetRepository)
	}
	if len(required) == 0 {
		if validation.PreviewID != preview.PreviewID {
			return required, kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
		}
		if validation.Outcome != "PASSED" {
			return required, kernel.Fail(kernel.ErrValidationBasisMismatch, "validation did not pass")
		}
	} else {
		ev := []gate.Evidence{}
		if cp.mergeEvidence != nil {
			ev = append(ev, cp.mergeEvidence(preview.PreviewID)...)
		}
		if validation.ReportID != "" {
			ev = append(ev, gate.Evidence{
				Name:    validation.SuiteRevision,
				BasisID: validation.PreviewID,
				Outcome: validation.Outcome,
			})
		}
		if err := gate.Check(required, gate.OnBasis(ev, preview.PreviewID)); err != nil {
			return required, err
		}
	}
	return required, nil
}

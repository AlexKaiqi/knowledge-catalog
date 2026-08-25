package controlplane

import (
	"kc/gate"
	"kc/kernel"
	"kc/snapshot"
)

func (cp *ControlPlane) Merge(proposal Proposal, preview Preview, validation ValidationReport) (commitID kernel.CommitID, err error) {
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
	required := []string{}
	if cp.mergeRequired != nil {
		required = cp.mergeRequired(proposal.TargetRepository)
	}
	if len(required) == 0 {
		if validation.PreviewID != preview.PreviewID {
			return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
		}
		if validation.Outcome != "PASSED" {
			return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation did not pass")
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
			return "", err
		}
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

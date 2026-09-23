package controlplane

import (
	"maps"

	"kc/catalog"
	"kc/kernel"
)

type Preview struct {
	PreviewID    string                                  `json:"previewId"`
	SetID        string                                  `json:"setId"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	BaseCommit   kernel.CommitID                         `json:"baseCommit"`
	Candidate    PreviewCandidate                        `json:"candidate"`
}

type PreviewCandidate struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	CommitID     kernel.CommitID     `json:"commitId"`
}

func (cp *ControlPlane) CreatePreview(setID string, proposal Proposal) (Preview, error) {
	resolved, err := cp.catalog.ResolveKnowledgeSet(setID)
	if err != nil {
		return Preview{}, err
	}
	if resolved.Repositories[proposal.TargetRepository] != proposal.BaseCommit {
		return Preview{}, kernel.Fail(kernel.ErrValidationBasisMismatch, "proposal base is not the current dataset member; re-pin after write")
	}
	overlaid, err := cp.catalog.ResolveKnowledgeSetOverlay(setID, map[kernel.RepositoryID]kernel.CommitID{
		proposal.TargetRepository: proposal.CandidateCommit,
	})
	if err != nil {
		return Preview{}, cp.note("preview", map[string]any{"proposalId": proposal.ProposalID, "dataset": setID}, err)
	}
	preview := Preview{
		PreviewID:    "preview-" + overlaid.PinID,
		SetID:        setID,
		Repositories: overlaid.Repositories,
		BaseCommit:   proposal.BaseCommit,
		Candidate: PreviewCandidate{
			RepositoryID: proposal.TargetRepository,
			CommitID:     proposal.CandidateCommit,
		},
	}
	return preview, cp.note("preview", map[string]any{"previewId": preview.PreviewID, "dataset": setID}, nil)
}

// CreatePreviewAt overlays a proposal on one caller-supplied, already fixed
// task basis. Governance must not resolve a named Workspace again.
func (cp *ControlPlane) CreatePreviewAt(resolved catalog.ResolvedKnowledgeSet, proposal Proposal) (Preview, error) {
	if resolved.Repositories[proposal.TargetRepository] != proposal.BaseCommit {
		return Preview{}, kernel.Fail(kernel.ErrValidationBasisMismatch, "proposal base is not the pinned dataset member; re-pin after write")
	}
	repo, ok := cp.store.Get(proposal.TargetRepository)
	if !ok || !repo.HasCommit(proposal.CandidateCommit) {
		return Preview{}, kernel.Fail(kernel.ErrVersionUnresolved, "proposal candidate is unavailable")
	}
	repositories := maps.Clone(resolved.Repositories)
	repositories[proposal.TargetRepository] = proposal.CandidateCommit
	preview := Preview{
		PreviewID: "preview-" + string(kernel.CanonicalDigest(struct {
			PinID      string
			Repository kernel.RepositoryID
			Candidate  kernel.CommitID
		}{resolved.PinID, proposal.TargetRepository, proposal.CandidateCommit})),
		SetID:        resolved.SetID,
		Repositories: repositories,
		BaseCommit:   proposal.BaseCommit,
		Candidate: PreviewCandidate{
			RepositoryID: proposal.TargetRepository,
			CommitID:     proposal.CandidateCommit,
		},
	}
	return preview, cp.note("preview", map[string]any{"previewId": preview.PreviewID, "pinId": resolved.PinID}, nil)
}

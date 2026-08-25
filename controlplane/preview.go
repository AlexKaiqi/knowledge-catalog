package controlplane

import "kc/kernel"

type Preview struct {
	PreviewID    string                                  `json:"previewId"`
	WorkspaceID  string                                  `json:"workspaceId"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	BaseCommit   kernel.CommitID                         `json:"baseCommit"`
	Candidate    PreviewCandidate                        `json:"candidate"`
}

type PreviewCandidate struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	CommitID     kernel.CommitID     `json:"commitId"`
}

func (cp *ControlPlane) CreatePreview(workspaceID string, proposal Proposal) (Preview, error) {
	resolved, err := cp.catalog.ResolveWorkspace(workspaceID)
	if err != nil {
		return Preview{}, err
	}
	if resolved.Repositories[proposal.TargetRepository] != proposal.BaseCommit {
		return Preview{}, kernel.Fail(kernel.ErrValidationBasisMismatch, "proposal base is not the current workspace member")
	}
	overlaid, err := cp.catalog.ResolveWorkspaceOverlay(workspaceID, map[kernel.RepositoryID]kernel.CommitID{
		proposal.TargetRepository: proposal.CandidateCommit,
	})
	if err != nil {
		return Preview{}, cp.note("preview", map[string]any{"proposalId": proposal.ProposalID, "workspace": workspaceID}, err)
	}
	preview := Preview{
		PreviewID:    "preview-" + overlaid.PinID,
		WorkspaceID:  workspaceID,
		Repositories: overlaid.Repositories,
		BaseCommit:   proposal.BaseCommit,
		Candidate: PreviewCandidate{
			RepositoryID: proposal.TargetRepository,
			CommitID:     proposal.CandidateCommit,
		},
	}
	return preview, cp.note("preview", map[string]any{"previewId": preview.PreviewID, "workspace": workspaceID}, nil)
}

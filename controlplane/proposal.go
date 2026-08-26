package controlplane

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type Proposal struct {
	ProposalID       string              `json:"proposalId"`
	TargetRepository kernel.RepositoryID `json:"targetRepository"`
	TargetRef        string              `json:"targetRef"`
	CandidateRef     string              `json:"candidateRef"`
	BaseCommit       kernel.CommitID     `json:"baseCommit"`
	CandidateCommit  kernel.CommitID     `json:"candidateCommit"`
	Rationale        string              `json:"rationale,omitempty"`
}

type ProposeInput struct {
	ProposalID   string
	RepositoryID kernel.RepositoryID
	TargetRef    string
	CandidateRef string
	BaseCommit   kernel.CommitID
	Operations   []knowledge.Operation
	Rationale    string
	Provenance   *knowledge.ProvenanceEnvelope
}

func (cp *ControlPlane) Propose(input ProposeInput) (Proposal, error) {
	base := input.BaseCommit
	if base == "" {
		repo, err := cp.store.Require(input.RepositoryID, kernel.ErrTargetRepositoryDenied)
		if err != nil {
			return Proposal{}, err
		}
		targetRef := snapshot.RefOrDefault(input.TargetRef)
		head, err := repo.Head(targetRef)
		if err != nil {
			return Proposal{}, err
		}
		base = head
	}
	receipt, err := cp.writer.Propose("proposal:"+input.ProposalID, writer.ProposeIntent{
		TargetRepository: input.RepositoryID,
		TargetRef:        input.TargetRef,
		CandidateRef:     input.CandidateRef,
		BaseCommit:       base,
		Operations:       input.Operations,
		Message:          input.Rationale,
		Provenance:       input.Provenance,
	})
	if err != nil {
		return Proposal{}, cp.note("propose", map[string]any{"proposalId": input.ProposalID, "repositoryId": string(input.RepositoryID)}, err)
	}
	proposal := Proposal{
		ProposalID:       input.ProposalID,
		TargetRepository: input.RepositoryID,
		TargetRef:        input.TargetRef,
		CandidateRef:     input.CandidateRef,
		BaseCommit:       base,
		CandidateCommit:  receipt.Result.CommitID,
		Rationale:        input.Rationale,
	}
	return proposal, cp.note("propose", map[string]any{
		"proposalId":      proposal.ProposalID,
		"repositoryId":    string(proposal.TargetRepository),
		"candidateCommit": string(proposal.CandidateCommit),
	}, nil)
}

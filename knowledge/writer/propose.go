package writer

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// PROPOSAL writes the same ChangeSet onto a candidate Ref. The target Ref
// (default main) does not move; merge is ControlPlane, not Writer.

// ProposeIntent writes a ChangeSet onto a candidate Ref. The target Ref does not move.
type ProposeIntent struct {
	TargetRepository kernel.RepositoryID
	TargetRef        string
	CandidateRef     string
	BaseCommit       kernel.CommitID
	Operations       []knowledge.Operation
	Message          string
	Provenance       *knowledge.ProvenanceEnvelope
}

func (w *Writer) Propose(commandID string, intent ProposeIntent) (CommitReceipt, error) {
	if intent.TargetRepository == "" || intent.CandidateRef == "" {
		return CommitReceipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "propose requires a target repository and candidate ref")
	}
	if prior, ok := w.Lookup(commandID); ok && prior.Request.Kind == string(knowledge.SurfaceProposal) {
		stored := prior.Request
		return w.applySnapshot(commandID, knowledge.SurfaceProposal, knowledge.CommitChangeSet{
			TargetRepository:     intent.TargetRepository,
			TargetRef:            stored.TargetRef,
			BaseCommit:           stored.BaseCommit,
			ExpectedTargetCommit: stored.ExpectedTargetCommit,
			Operations:           intent.Operations,
			Message:              intent.Message,
			Provenance:           intent.Provenance,
		})
	}
	repo, err := w.store.Require(intent.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return CommitReceipt{}, err
	}
	if repo.Archived() {
		return CommitReceipt{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", intent.TargetRepository)
	}
	targetRef := snapshot.RefOrDefault(intent.TargetRef)
	parent := intent.BaseCommit
	if existing, ok := repo.GetRef(intent.CandidateRef); ok {
		parent = existing
	} else {
		if parent == "" {
			parent, err = repo.Head(targetRef)
			if err != nil {
				return CommitReceipt{}, err
			}
		}
		if err := repo.CreateRef(intent.CandidateRef, parent); err != nil {
			return CommitReceipt{}, err
		}
	}
	return w.applySnapshot(commandID, knowledge.SurfaceProposal, knowledge.CommitChangeSet{
		TargetRepository:     intent.TargetRepository,
		TargetRef:            intent.CandidateRef,
		BaseCommit:           parent,
		ExpectedTargetCommit: parent,
		Operations:           intent.Operations,
		Message:              intent.Message,
		Provenance:           intent.Provenance,
	})
}

package writer

import (
	"kc/kernel"
	"kc/knowledge"
)

// COMMIT applies a ChangeSet and CAS-moves the target Ref (default main).
// CommitIntent fills expectedTargetCommit from the current head on first use;
// retries reuse the stored CAS. applySnapshot is shared with PROPOSAL.

// CommitIntent is COMMIT before CAS is filled from the current Ref.
type CommitIntent struct {
	TargetRepository     kernel.RepositoryID
	TargetRef            string
	BaseCommit           kernel.CommitID
	ExpectedTargetCommit kernel.CommitID
	Operations           []knowledge.Operation
	Message              string
	Provenance           *knowledge.ProvenanceEnvelope
}

func (w *Writer) CommitIntent(commandID string, intent CommitIntent) (CommitReceipt, error) {
	if prior, ok := w.Lookup(commandID); ok && prior.Request.Kind == string(knowledge.SurfaceCommit) && prior.Request.ChangeSet != nil {
		stored := prior.Request.ChangeSet
		return w.Commit(commandID, knowledge.CommitChangeSet{
			TargetRepository:     intent.TargetRepository,
			TargetRef:            intent.TargetRef,
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
	head, err := repo.Head(intent.TargetRef)
	if err != nil {
		return CommitReceipt{}, err
	}
	base := intent.BaseCommit
	expected := intent.ExpectedTargetCommit
	if expected == "" {
		expected = head
	}
	if base == "" {
		base = expected
	}
	return w.Commit(commandID, knowledge.CommitChangeSet{
		TargetRepository:     intent.TargetRepository,
		TargetRef:            intent.TargetRef,
		BaseCommit:           base,
		ExpectedTargetCommit: expected,
		Operations:           intent.Operations,
		Message:              intent.Message,
		Provenance:           intent.Provenance,
	})
}

func (w *Writer) Commit(commandID string, cs knowledge.CommitChangeSet) (CommitReceipt, error) {
	return w.applySnapshot(commandID, knowledge.SurfaceCommit, cs)
}

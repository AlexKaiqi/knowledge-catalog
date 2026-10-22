package writer

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// COMMIT applies a ChangeSet and CAS-moves the target Ref (default main).
// CommitIntent fills expectedTargetCommit from the current head on first use;
// retries reuse the stored CAS. A branch Ref is an ordinary target; see merge.go.

// CommitIntent is COMMIT before CAS is filled from the current Ref.
type CommitIntent struct {
	TargetRepository     kernel.RepositoryID
	TargetRef            string
	BaseCommit           kernel.CommitID
	ExpectedTargetCommit kernel.CommitID
	Operations           []objectmodel.Operation
	Message              string
	Provenance           *objectmodel.ProvenanceEnvelope
	BulkIngest           bool
}

func (w *Writer) CommitIntent(commandID string, intent CommitIntent) (CommitReceipt, error) {
	if entry, ok := w.commands.Lookup(commandID); ok && entry.Status == snapshot.StatusApplied {
		request := entry.Request
		if request.Kind == "" || request.RepositoryID == "" || request.TargetRef == "" || request.BaseCommit == "" || request.ExpectedTargetCommit == "" || request.OperationCount < 1 || len(entry.Receipt) == 0 {
			return CommitReceipt{}, kernel.Fail(kernel.ErrPreconditionFailed, "command %s has incomplete original write material", commandID)
		}
	}
	if prior, ok := w.Lookup(commandID); ok && prior.Request.Kind == string(objectmodel.SurfaceCommit) {
		stored := prior.Request
		if intent.BaseCommit != "" && intent.BaseCommit != stored.BaseCommit || intent.ExpectedTargetCommit != "" && intent.ExpectedTargetCommit != stored.ExpectedTargetCommit {
			return CommitReceipt{}, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s has a different explicit write basis", commandID)
		}
		return w.Commit(commandID, objectmodel.CommitChangeSet{
			TargetRepository:     intent.TargetRepository,
			TargetRef:            intent.TargetRef,
			BaseCommit:           stored.BaseCommit,
			ExpectedTargetCommit: stored.ExpectedTargetCommit,
			Operations:           intent.Operations,
			Message:              intent.Message,
			Provenance:           intent.Provenance,
			BulkIngest:           intent.BulkIngest,
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
	return w.Commit(commandID, objectmodel.CommitChangeSet{
		TargetRepository:     intent.TargetRepository,
		TargetRef:            intent.TargetRef,
		BaseCommit:           base,
		ExpectedTargetCommit: expected,
		Operations:           intent.Operations,
		Message:              intent.Message,
		Provenance:           intent.Provenance,
		BulkIngest:           intent.BulkIngest,
	})
}

func (w *Writer) Commit(commandID string, cs objectmodel.CommitChangeSet) (CommitReceipt, error) {
	return w.applySnapshot(commandID, objectmodel.SurfaceCommit, cs)
}

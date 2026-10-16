package invocation

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

// A directory retry must reproduce the accepted request, not merely match
// today's content. Writer then checks the rebuilt request's durable digest.
func rebuildDesiredReplay(cx *Invocation, commandID string, desired objectmodel.CommitChangeSet) (objectmodel.CommitChangeSet, error) {
	if cx.WS.Commands == nil {
		return objectmodel.CommitChangeSet{}, kernel.Fail(kernel.ErrPreconditionFailed, "cannot verify directory retry %s without the command ledger", commandID)
	}
	entry, ok := cx.WS.Commands.Lookup(commandID)
	if !ok {
		return objectmodel.CommitChangeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "desired state already matches the current version")
	}
	request := entry.Request
	if entry.Status != snapshot.StatusApplied || entry.Digest == "" || len(entry.Receipt) == 0 ||
		request.BaseCommit == "" || request.ExpectedTargetCommit == "" || request.OperationCount < 1 {
		return objectmodel.CommitChangeSet{}, kernel.Fail(kernel.ErrPreconditionFailed, "cannot verify directory retry %s without its applied request basis", commandID)
	}
	if request.Kind != string(objectmodel.SurfaceCommit) || request.RepositoryID != string(desired.TargetRepository) || request.TargetRef != desired.TargetRef ||
		(cx.Flag("base") != "" && cx.Flag("base") != request.BaseCommit) {
		return objectmodel.CommitChangeSet{}, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s has a different target or explicit basis", commandID)
	}
	desired.BaseCommit = kernel.CommitID(request.BaseCommit)
	desired.ExpectedTargetCommit = kernel.CommitID(request.ExpectedTargetCommit)
	current, err := CurrentDigests(cx.WS.Reader, desired.TargetRepository, desired.BaseCommit, desired.Operations)
	if err != nil {
		return objectmodel.CommitChangeSet{}, err
	}
	rebuilt, _ := writer.OmitUnchanged(desired, current)
	if len(rebuilt.Operations) == 0 {
		return objectmodel.CommitChangeSet{}, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s has a different directory request", commandID)
	}
	return rebuilt, nil
}

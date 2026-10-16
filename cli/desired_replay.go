package cli

import (
	"context"

	kcclient "kc/client"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	objectcli "kc/object/cli"
	"kc/object/writer"
	"kc/snapshot"
)

// A matching current value does not prove that this command published it.
// Rebuild the directory's original comparison and let Writer verify the full
// request digest, including its target, declarations, provenance and bulk mode.
func replayRemoteDesired(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, commandID string, desired objectmodel.CommitChangeSet, options kcclient.RequestOptions) (any, error) {
	var entry snapshot.CommandEntry
	if err := client.WriterService().Receipt(ctx, commandID, options, &entry); err != nil {
		return nil, err
	}
	stored := entry.Request
	if entry.CommandID != commandID || entry.Status != snapshot.StatusApplied || entry.Digest == "" ||
		len(entry.Receipt) == 0 || stored.BaseCommit == "" || stored.ExpectedTargetCommit == "" || stored.OperationCount < 1 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "cannot verify directory retry %s without an applied command and its original basis", commandID)
	}
	if stored.Kind != string(objectmodel.SurfaceCommit) || stored.RepositoryID != string(desired.TargetRepository) || stored.TargetRef != desired.TargetRef ||
		(invoke.FlagString(flags, "base") != "" && invoke.FlagString(flags, "base") != stored.BaseCommit) {
		return nil, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s was accepted with a different target or basis", commandID)
	}
	desired.BaseCommit = kernel.CommitID(stored.BaseCommit)
	desired.ExpectedTargetCommit = kernel.CommitID(stored.ExpectedTargetCommit)
	current, err := remoteCurrentDigests(ctx, client, string(desired.TargetRepository), desired.BaseCommit, desired.Operations, options)
	if err != nil {
		return nil, err
	}
	rebuilt, _ := writer.OmitUnchanged(desired, current)
	if len(rebuilt.Operations) == 0 {
		return nil, kernel.Fail(kernel.ErrIdempotencyConflict, "command %s was accepted with a different directory request", commandID)
	}
	return remoteCommitWithReceiptRecovery(ctx, client, string(desired.TargetRepository),
		kcclient.CommitRequest{CommandID: commandID, ChangeSet: rebuilt}, options, objectcli.ReceiptWaitBudget())
}

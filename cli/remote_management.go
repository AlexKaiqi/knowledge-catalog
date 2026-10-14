package cli

import (
	"context"
	inv "kc/invocation"

	kcclient "kc/client"
	"kc/invoke"
	"kc/kernel"
	objectcli "kc/object/cli"
	"kc/object/writer"
)

func runRemoteWriter(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	switch path {
	case "writer commit":
		if invoke.FlagString(flags, "dir") != "" {
			return runRemoteDesiredCommit(ctx, client, flags, options)
		}
		request, repository, err := remoteCommitRequest(path, flags)
		if err != nil {
			return nil, err
		}
		return remoteCommitWithReceiptRecovery(ctx, client, repository, request, options, objectcli.ReceiptWaitBudget())
	case "writer head":
		repository, err := inv.RequireRemoteFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		err = client.WriterService().Head(ctx, repository, snapshotRef(flags), options, &output)
		return output, err
	case "writer branch":
		repository, err := inv.RequireRemoteFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		name, err := inv.RequireRemoteFlag(flags, "name")
		if err != nil {
			return nil, err
		}
		if invoke.FlagString(flags, "from") != "" && invoke.FlagString(flags, "commit") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --from or --commit")
		}
		request := kcclient.BranchRequest{Name: name, From: invoke.FlagString(flags, "from"), Commit: invoke.FlagString(flags, "commit")}
		err = client.WriterService().Branch(ctx, repository, request, options, &output)
		return output, err
	case "writer merge":
		repository, err := inv.RequireRemoteFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		commandID, err := inv.RequireRemoteFlag(flags, "command-id")
		if err != nil {
			return nil, err
		}
		from, err := inv.RequireRemoteFlag(flags, "from")
		if err != nil {
			return nil, err
		}
		request := kcclient.MergeRequest{CommandID: commandID, From: from, Into: invoke.FlagString(flags, "into"), Commit: invoke.FlagString(flags, "commit"), Expected: invoke.FlagString(flags, "expected")}
		err = client.WriterService().Merge(ctx, repository, request, options, &output)
		return output, err
	case "writer receipt":
		commandID, err := inv.RequireRemoteFlag(flags, "command-id")
		if err != nil {
			return nil, err
		}
		err = client.WriterService().Receipt(ctx, commandID, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteDesiredCommit(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	commandID, err := inv.RequireRemoteFlag(flags, "command-id")
	if err != nil {
		return nil, err
	}
	repository, err := inv.RequireRemoteFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	dir, err := inv.RequireRemoteFlag(flags, "dir")
	if err != nil {
		return nil, err
	}
	preview, _, current, err := prepareRemoteDesiredIngest(ctx, client, flags, repository, dir, options)
	if err != nil {
		return nil, err
	}
	changeSet, _ := writer.OmitUnchanged(preview.ChangeSet, current)
	if len(changeSet.Operations) == 0 {
		if receipt, ok, _ := remoteReceipt(ctx, client, commandID, options); ok {
			receipt.Disposition = writer.DispositionReplayed
			return receipt, nil
		}
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "desired state already matches the current version")
	}
	return remoteCommitWithReceiptRecovery(ctx, client, repository,
		kcclient.CommitRequest{CommandID: commandID, ChangeSet: changeSet}, options, objectcli.ReceiptWaitBudget())
}

func runRemoteDesiredDiff(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if invoke.FlagString(flags, "changeset") != "" || invoke.FlagString(flags, "payload") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "diff uses --dir, not --changeset")
	}
	repository, err := inv.RequireRemoteFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	dir, err := inv.RequireRemoteFlag(flags, "dir")
	if err != nil {
		return nil, err
	}
	preview, base, current, err := prepareRemoteDesiredIngest(ctx, client, flags, repository, dir, options)
	if err != nil {
		return nil, err
	}
	return inv.DesiredDiffResult(repository, base, preview.ChangeSet.Operations, current), nil
}

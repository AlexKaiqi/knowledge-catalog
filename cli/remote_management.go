package cli

import (
	"context"
	inv "kc/invocation"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge/writer"
)

func runRemoteWriter(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	switch path {
	case "writer put", "writer remove":
		request, repository, err := remoteCommitRequest(path, flags)
		if err != nil {
			return nil, err
		}
		return remoteCommitWithReceiptRecovery(ctx, client, repository, request, options, commitReceiptWaitBudget())
	case "writer commit":
		if flagString(flags, "dir") != "" {
			return runRemoteDesiredCommit(ctx, client, flags, options)
		}
		request, repository, err := remoteCommitRequest(path, flags)
		if err != nil {
			return nil, err
		}
		return remoteCommitWithReceiptRecovery(ctx, client, repository, request, options, commitReceiptWaitBudget())
	case "writer head":
		repository, err := requireRemoteFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		err = client.WriterService().Head(ctx, repository, snapshotRef(flags), options, &output)
		return output, err
	case "writer receipt":
		commandID, err := requireRemoteFlag(flags, "command-id")
		if err != nil {
			return nil, err
		}
		err = client.WriterService().Receipt(ctx, commandID, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteGovernance(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	service := client.GovernanceService()
	switch path {
	case "governance proposal create":
		request, err := remoteProposalRequest(flags)
		if err != nil {
			return nil, err
		}
		err = service.Proposal(ctx, request, options, &output)
		return output, err
	case "governance preview create":
		if flagString(flags, "dataset") == "" && len(remotePinDocument(flags)) == 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "governance preview create requires --dataset")
		}
		request := kcclient.PreviewRequest{Dataset: flagString(flags, "dataset"), Pin: remotePinDocument(flags), Proposal: flagString(flags, "proposal")}
		err := service.Preview(ctx, request, options, &output)
		return output, err
	case "governance preview validate":
		request := kcclient.ValidateRequest{Preview: flagString(flags, "preview")}
		err := service.Validate(ctx, request, options, &output)
		return output, err
	case "governance validation record":
		request := kcclient.ValidationRequest{Preview: flagString(flags, "preview"), Suite: flagString(flags, "suite"), Outcome: flagString(flags, "outcome")}
		err := service.RecordValidation(ctx, request, options, &output)
		return output, err
	case "governance proposal merge":
		request := kcclient.MergeRequest{Proposal: flagString(flags, "proposal"), Preview: flagString(flags, "preview"), Validation: flagString(flags, "validation")}
		err := service.Merge(ctx, request, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteDesiredCommit(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	commandID, err := requireRemoteFlag(flags, "command-id")
	if err != nil {
		return nil, err
	}
	repository, err := requireRemoteFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	dir, err := requireRemoteFlag(flags, "dir")
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
		kcclient.CommitRequest{CommandID: commandID, ChangeSet: changeSet}, options, commitReceiptWaitBudget())
}

func runRemoteDesiredDiff(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if flagString(flags, "changeset") != "" || flagString(flags, "payload") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "diff uses --dir, not --changeset")
	}
	repository, err := requireRemoteFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	dir, err := requireRemoteFlag(flags, "dir")
	if err != nil {
		return nil, err
	}
	preview, base, current, err := prepareRemoteDesiredIngest(ctx, client, flags, repository, dir, options)
	if err != nil {
		return nil, err
	}
	return inv.DesiredDiffResult(repository, base, preview.ChangeSet.Operations, current), nil
}

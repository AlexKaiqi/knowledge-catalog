package cli

import (
	"context"
	inv "kc/invocation"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge/writer"
	"kc/snapshot/dataset"
)

func runRemoteRegistry(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	service := client.SnapshotService()
	if path == "create" {
		if err := validateManagedRepositoryCreateFlags(flags); err != nil {
			return nil, err
		}
		var output any
		var err error
		if flagString(flags, "name") != "" {
			err = service.CreateNamedRepository(ctx, kcclient.NamedRepositoryCreateRequest{
				Name: flagString(flags, "name"), Store: flagString(flags, "store"),
			}, options, &output)
		} else {
			repository, deriveErr := repositoryIDFromConnectionURL(flagString(flags, "url"))
			if deriveErr != nil {
				return nil, deriveErr
			}
			credential, credentialErr := connectionCredentialFile(flags)
			if credentialErr != nil {
				return nil, credentialErr
			}
			err = service.ConnectRepository(ctx, kcclient.ConnectionRequest{
				Repository: repository, Driver: "lakefs", URL: flagString(flags, "url"), Credential: credential,
			}, options, &output)
		}
		return output, err
	}
	var err error
	var output any
	switch path {
	case "show":
		err = service.Show(ctx, options, &output)
	case "registry audit":
		var limit int
		limit, err = remoteLimit(flags)
		if err == nil {
			err = service.Audit(ctx, limit, flagString(flags, "continuation"), options, &output)
		}
	case "attach":
		err = service.AttachRepository(ctx, kcclient.RepositoryAttachRequest{Repository: flagString(flags, "repo")}, options, &output)
	case "detach":
		err = service.DetachRepository(ctx, flagString(flags, "repo"), options, &output)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
	return output, err
}

func runRemoteWorkspace(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if path == "pin" && flagString(flags, "dataset") == "" && flagString(flags, "source") == "" && flagString(flags, "file") == "" && flagString(flags, "from-repo") == "" && flagString(flags, "payload") == "" && len(flagStrings(flags, "file-source")) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc pin requires --dataset <id> or --source <repository>[=selector]...")
	}
	service := client.SnapshotService()
	var err error
	var output any
	switch path {
	case "pin":
		if flagString(flags, "dataset") == "" {
			output, err = runRemoteWorkspaceResolveDefinition(ctx, service, flags, options)
		} else {
			var resolved dataset.ResolvedKnowledgeSet
			err = service.ResolveKnowledgeSet(ctx, flagString(flags, "dataset"), kcclient.KnowledgeSetResolveRequest{Pin: remotePin(flags)}, options, &resolved)
			output = taskKnowledgeSetPin{ResolvedKnowledgeSet: resolved}
		}
		if err != nil {
			return nil, err
		}
		return inv.ShapePinOutput(flags, output)
	case "pin check":
		err = service.CheckKnowledgeSet(ctx, flagString(flags, "dataset"), kcclient.KnowledgeSetResolveRequest{Pin: remotePin(flags)}, options, &output)
	case "dataset define":
		return runRemoteWorkspaceDefine(ctx, service, flags, options)
	case "dataset clone":
		return runRemoteDatasetClone(ctx, client, flags, options)
	case "dataset retire":
		err = service.RetireKnowledgeSet(ctx, flagString(flags, "dataset"), options, &output)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
	return output, err
}

func runRemoteWorkspaceResolveDefinition(ctx context.Context, service kcclient.SnapshotService, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	sources, err := remoteKnowledgeSetSources(flags)
	if err != nil {
		return nil, err
	}
	revision := 1
	if flagString(flags, "revision") != "" {
		revision, err = remoteIntFlag(flags, "revision")
		if err != nil {
			return nil, err
		}
	}
	definition := dataset.KnowledgeSet{Revision: revision, Sources: sources}
	var output dataset.ResolvedKnowledgeSet
	err = service.ResolveDefinition(ctx, kcclient.KnowledgeSetRequest{
		Dataset: flagString(flags, "dataset"), Revision: revision, Sources: sources,
	}, options, &output)
	if err != nil {
		return nil, err
	}
	return taskKnowledgeSetPin{ResolvedKnowledgeSet: output, Definition: &definition}, nil
}

func runRemoteWorkspaceDefine(ctx context.Context, service kcclient.SnapshotService, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	workspace, err := requireRemoteFlag(flags, "dataset")
	if err != nil {
		return nil, err
	}
	revision, err := remoteIntFlag(flags, "revision")
	if err != nil {
		return nil, err
	}
	sources, err := remoteKnowledgeSetSources(flags)
	if err != nil {
		return nil, err
	}
	var output any
	err = service.DefineKnowledgeSet(ctx, kcclient.KnowledgeSetRequest{
		Dataset: workspace, Revision: revision, Sources: sources,
	}, options, &output)
	return output, err
}

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

func runRemoteAdmin(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	service := client.AdminService()
	switch path {
	case "grant add", "admin grant add":
		request := kcclient.GrantRequest{Principal: flagString(flags, "principal"), Actions: splitCmds(flagString(flags, "action")), Repository: flagString(flags, "repo"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Dataset: flagString(flags, "dataset")}
		err := service.AddGrant(ctx, request, options, &output)
		return output, err
	case "grant list", "admin grant list":
		return runRemoteGrantList(ctx, client, flags, options)
	case "grant remove", "admin grant remove":
		err := service.RemoveGrant(ctx, flagString(flags, "id"), options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteGrantList(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	action := flagString(flags, "action")
	principal := flagString(flags, "principal")
	filter := kcclient.GrantRequest{Principal: principal, Repository: flagString(flags, "repo"), Dataset: flagString(flags, "dataset"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect")}
	if action != "" {
		if principal == "" {
			identity, ok, err := client.Identity(ctx)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, kernel.Fail(kernel.ErrUnauthenticated, "grant question requires an authenticated principal")
			}
			principal = identity.Principal
		}
		// Keep the CLI's existing explicit-rule question, including global rules.
		// It consequently requires a global grant inventory read capability.
		filter = kcclient.GrantRequest{Principal: principal, Actions: []string{action}}
	}
	var listed inv.GrantList
	if err := client.AdminService().GrantsMatching(ctx, filter, options, &listed); err != nil {
		return nil, err
	}
	if action == "" {
		return listed, nil
	}
	return inv.EvaluateAllow(listed.Rules, inv.AllowQuery{Principal: principal, Action: action, Repo: flagString(flags, "repo"), Dataset: flagString(flags, "dataset"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect")})
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

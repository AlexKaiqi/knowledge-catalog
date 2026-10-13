package cli

import (
	"context"

	kcclient "kc/client"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot/dataset"
)

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
	definition, err := remoteDatasetInput(flags)
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
	definition.Revision = revision
	var output dataset.ResolvedKnowledgeSet
	err = service.ResolveDefinition(ctx, kcclient.KnowledgeSetRequest{
		Dataset: flagString(flags, "dataset"), Revision: revision, Sources: definition.Sources, Metadata: definition.Metadata,
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
	definition, err := remoteDatasetInput(flags)
	if err != nil {
		return nil, err
	}
	var output any
	err = service.DefineKnowledgeSet(ctx, kcclient.KnowledgeSetRequest{
		Dataset: workspace, Revision: revision, Sources: definition.Sources, Metadata: definition.Metadata,
	}, options, &output)
	return output, err
}

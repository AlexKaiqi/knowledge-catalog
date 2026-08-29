package cli

import (
	"context"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge/writer"
)

func runRemoteCatalog(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	catalogID, err := requireRemoteFlag(flags, "catalog")
	if err != nil {
		return nil, err
	}
	var output any
	service := client.CatalogService()
	switch path {
	case "catalog show":
		err = service.Show(ctx, catalogID, options, &output)
	case "catalog audit":
		var limit int
		limit, err = remoteLimit(flags)
		if err == nil {
			err = service.Audit(ctx, catalogID, limit, options, &output)
		}
	case "catalog archive":
		err = service.Archive(ctx, catalogID, options, &output)
	case "catalog repository list":
		err = service.Repositories(ctx, catalogID, options, &output)
	case "catalog repository register":
		err = service.RegisterRepository(ctx, catalogID, kcclient.RepositoryRegisterRequest{Repository: FlagString(flags, "repo")}, options, &output)
	case "catalog repository archive":
		err = service.ArchiveRepository(ctx, catalogID, FlagString(flags, "repo"), options, &output)
	case "catalog workspace list":
		err = service.Workspaces(ctx, catalogID, options, &output)
	case "catalog workspace show":
		err = service.Workspace(ctx, catalogID, FlagString(flags, "workspace"), options, &output)
	case "catalog workspace retire":
		err = service.RetireWorkspace(ctx, catalogID, FlagString(flags, "workspace"), options, &output)
	case "catalog workspace resolve":
		err = service.ResolveWorkspace(ctx, catalogID, FlagString(flags, "workspace"), kcclient.WorkspaceResolveRequest{Pin: remotePin(flags)}, options, &output)
	case "catalog workspace check":
		err = service.CheckWorkspace(ctx, catalogID, FlagString(flags, "workspace"), kcclient.WorkspaceResolveRequest{Pin: remotePin(flags)}, options, &output)
	case "catalog workspace define":
		return runRemoteWorkspaceDefine(ctx, service, catalogID, flags, options)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
	return output, err
}

func runRemoteWorkspaceDefine(ctx context.Context, service kcclient.CatalogService, catalogID string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	workspace, err := requireRemoteFlag(flags, "workspace")
	if err != nil {
		return nil, err
	}
	revision, err := remoteIntFlag(flags, "revision")
	if err != nil {
		return nil, err
	}
	sources, err := remoteWorkspaceSources(flags)
	if err != nil {
		return nil, err
	}
	var output any
	err = service.DefineWorkspace(ctx, catalogID, kcclient.WorkspaceDefinitionRequest{
		Workspace: workspace, Revision: revision, Sources: sources,
	}, options, &output)
	return output, err
}

func runRemoteWriter(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	switch path {
	case "writer put", "writer remove", "writer commit":
		request, repository, err := remoteCommitRequest(path, flags)
		if err != nil {
			return nil, err
		}
		err = client.WriterService().Commit(ctx, repository, request, options, &output)
		return output, err
	case "writer ingest":
		repository, err := requireRemoteFlag(flags, "repo")
		if err != nil {
			return nil, err
		}
		dir, err := requireRemoteFlag(flags, "dir")
		if err != nil {
			return nil, err
		}
		return writer.Ingest(dir, kernel.RepositoryID(repository), kernel.CommitID(FlagString(flags, "base")))
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
		request := kcclient.PreviewRequest{Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Proposal: FlagString(flags, "proposal")}
		err := service.Preview(ctx, request, options, &output)
		return output, err
	case "governance preview validate":
		request := kcclient.ValidateRequest{Catalog: FlagString(flags, "catalog"), Preview: FlagString(flags, "preview")}
		err := service.Validate(ctx, request, options, &output)
		return output, err
	case "governance validation record":
		request := kcclient.ValidationRequest{Catalog: FlagString(flags, "catalog"), Preview: FlagString(flags, "preview"), Suite: FlagString(flags, "suite"), Outcome: FlagString(flags, "outcome")}
		err := service.RecordValidation(ctx, request, options, &output)
		return output, err
	case "governance proposal merge":
		request := kcclient.MergeRequest{Catalog: FlagString(flags, "catalog"), Proposal: FlagString(flags, "proposal"), Preview: FlagString(flags, "preview"), Validation: FlagString(flags, "validation")}
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
	case "admin grant add":
		request := kcclient.GrantRequest{Principal: FlagString(flags, "principal"), Actions: splitCmds(FlagString(flags, "action")), Repository: FlagString(flags, "repo"), Catalog: FlagString(flags, "catalog"), Ref: FlagString(flags, "ref"), Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Workspace: FlagString(flags, "workspace")}
		err := service.AddGrant(ctx, request, options, &output)
		return output, err
	case "admin grant list":
		err := service.Grants(ctx, options, &output)
		return output, err
	case "admin grant remove":
		err := service.RemoveGrant(ctx, FlagString(flags, "id"), options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

package cli

import (
	"context"
	"encoding/json"
	"strings"

	"kc/catalog"
	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteCatalog(ctx context.Context, client *kcclient.Client, server, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	service := client.CatalogService()
	if path == "catalog list" {
		var output any
		if err := service.Catalogs(ctx, options, &output); err != nil {
			return nil, err
		}
		var listed remoteCatalogList
		if raw, err := json.Marshal(output); err == nil {
			_ = json.Unmarshal(raw, &listed)
		}
		if len(listed.Catalogs) == 1 {
			_ = persistClientCatalog(server, listed.Catalogs[0].ID)
		}
		return output, nil
	}
	if path == "create" {
		if err := validateManagedRepositoryCreateFlags(flags); err != nil {
			return nil, err
		}
		catalogID, err := remoteCatalogID(ctx, server, service, flags, options)
		if err != nil {
			return nil, err
		}
		var output any
		if FlagString(flags, "name") != "" {
			err = service.CreateNamedRepository(ctx, kcclient.NamedRepositoryCreateRequest{
				Name: FlagString(flags, "name"), Store: FlagString(flags, "store"), Catalog: catalogID,
			}, options, &output)
		} else {
			repository, deriveErr := repositoryIDFromConnectionURL(FlagString(flags, "url"))
			if deriveErr != nil {
				return nil, deriveErr
			}
			credential, credentialErr := connectionCredentialFile(flags)
			if credentialErr != nil {
				return nil, credentialErr
			}
			err = service.ConnectRepository(ctx, catalogID, kcclient.ConnectionRequest{
				Repository: repository, Driver: "gitea", URL: FlagString(flags, "url"), Credential: credential,
			}, options, &output)
		}
		return output, err
	}
	catalogID, err := remoteCatalogID(ctx, server, service, flags, options)
	if err != nil {
		return nil, err
	}
	var output any
	switch path {
	case "show":
		err = service.Show(ctx, catalogID, options, &output)
	case "catalog audit":
		var limit int
		limit, err = remoteLimit(flags)
		if err == nil {
			err = service.Audit(ctx, catalogID, limit, options, &output)
		}
	case "catalog archive":
		err = service.Archive(ctx, catalogID, options, &output)
	case "attach":
		err = service.AttachRepository(ctx, catalogID, kcclient.RepositoryAttachRequest{Repository: FlagString(flags, "repo")}, options, &output)
	case "detach":
		err = service.DetachRepository(ctx, catalogID, FlagString(flags, "repo"), options, &output)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
	return output, err
}

func runRemoteWorkspace(ctx context.Context, client *kcclient.Client, server, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	service := client.CatalogService()
	catalogID, err := remoteCatalogID(ctx, server, service, flags, options)
	if err != nil {
		return nil, err
	}
	var output any
	switch path {
	case "workspace pin":
		if FlagString(flags, "workspace") == "" {
			output, err = runRemoteWorkspaceResolveDefinition(ctx, service, catalogID, flags, options)
		} else {
			var resolved catalog.ResolvedWorkspace
			err = service.ResolveWorkspace(ctx, catalogID, FlagString(flags, "workspace"), kcclient.WorkspaceResolveRequest{Pin: remotePin(flags)}, options, &resolved)
			output = taskWorkspacePin{ResolvedWorkspace: resolved, Catalog: catalogID}
		}
		if err != nil {
			return nil, err
		}
		return shapePinOutput(flags, output)
	case "workspace check":
		err = service.CheckWorkspace(ctx, catalogID, FlagString(flags, "workspace"), kcclient.WorkspaceResolveRequest{Pin: remotePin(flags)}, options, &output)
	case "workspace define":
		return runRemoteWorkspaceDefine(ctx, service, catalogID, flags, options)
	case "workspace retire":
		err = service.RetireWorkspace(ctx, catalogID, FlagString(flags, "workspace"), options, &output)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
	return output, err
}

type remoteCatalogList struct {
	Catalogs []struct {
		ID string `json:"id"`
	} `json:"catalogs"`
}

func verifyRemoteCatalogUse(ctx context.Context, service kcclient.CatalogService, catalogID string, options kcclient.RequestOptions) error {
	var listed remoteCatalogList
	if err := service.Catalogs(ctx, options, &listed); err != nil {
		if kernel.CodeOf(err) != kernel.ErrForbidden {
			return err
		}
		// Principals without catalog.read may still target one Catalog through
		// explicit management verbs; downstream commands enforce action grants.
		return nil
	}
	if len(listed.Catalogs) == 0 {
		// Mock/minimal servers and pre-admission clients may return an empty
		// inventory; persisting an explicit catalog id is still allowed.
		return nil
	}
	for _, item := range listed.Catalogs {
		if item.ID == catalogID {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "catalog %s is not visible to this principal", catalogID)
}

func remoteCatalogID(ctx context.Context, server string, service kcclient.CatalogService, flags map[string]FlagValue, options kcclient.RequestOptions) (string, error) {
	if id := strings.TrimSpace(FlagString(flags, "catalog")); id != "" {
		return id, nil
	}
	if id := savedClientCatalog(server); id != "" {
		return id, nil
	}
	var listed remoteCatalogList
	if err := service.Catalogs(ctx, options, &listed); err != nil {
		return "", err
	}
	if len(listed.Catalogs) == 1 {
		id := listed.Catalogs[0].ID
		_ = persistClientCatalog(server, id)
		return id, nil
	}
	if len(listed.Catalogs) == 0 {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "no visible catalog")
	}
	return "", kernel.Fail(kernel.ErrUsageInvalid, "catalog use is required when more than one catalog is visible")
}

func runRemoteWorkspaceResolveDefinition(ctx context.Context, service kcclient.CatalogService, catalogID string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	sources, err := remoteWorkspaceSources(flags)
	if err != nil {
		return nil, err
	}
	revision := 1
	if FlagString(flags, "revision") != "" {
		revision, err = remoteIntFlag(flags, "revision")
		if err != nil {
			return nil, err
		}
	}
	definition := catalog.WorkspaceDefinition{Revision: revision, Sources: sources}
	var output catalog.ResolvedWorkspace
	err = service.ResolveDefinition(ctx, catalogID, kcclient.WorkspaceDefinitionRequest{
		Workspace: FlagString(flags, "workspace"), Revision: revision, Sources: sources,
	}, options, &output)
	if err != nil {
		return nil, err
	}
	return taskWorkspacePin{ResolvedWorkspace: output, Catalog: catalogID, Definition: &definition}, nil
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
		pin := remotePinDocument(flags)
		if len(pin) == 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "governance preview create requires --pin")
		}
		request := kcclient.PreviewRequest{Pin: pin, Proposal: FlagString(flags, "proposal")}
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
	case "grant add", "admin grant add":
		request := kcclient.GrantRequest{Principal: FlagString(flags, "principal"), Actions: splitCmds(FlagString(flags, "action")), Repository: FlagString(flags, "repo"), Catalog: FlagString(flags, "catalog"), Ref: FlagString(flags, "ref"), Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Workspace: FlagString(flags, "workspace")}
		err := service.AddGrant(ctx, request, options, &output)
		return output, err
	case "grant list", "admin grant list":
		err := service.Grants(ctx, options, &output)
		return output, err
	case "grant remove", "admin grant remove":
		err := service.RemoveGrant(ctx, FlagString(flags, "id"), options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

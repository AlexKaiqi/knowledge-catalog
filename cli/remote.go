package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"kc/catalog"
	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
)

func remoteServerURL(flags map[string]FlagValue) string {
	if value := strings.TrimSpace(FlagString(flags, "server")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("KC_SERVER_URL"))
}

func runRemoteCLI(ctx context.Context, server, path string, flags map[string]FlagValue) RunResult {
	if _, explicitHome := flags["home"]; explicitHome {
		return errorResult(kernel.Fail(kernel.ErrUsageInvalid, "--server and --home are mutually exclusive"))
	}
	principal := strings.TrimSpace(FlagString(flags, "as"))
	if principal == "" {
		principal = strings.TrimSpace(os.Getenv("KC_AS"))
	}
	if principal == "" {
		return errorResult(kernel.Fail(kernel.ErrUnauthenticated, "remote kc requires an explicit principal or authenticated client session"))
	}
	authentication := strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN"))
	var authenticator kcclient.Authenticator
	if authentication != "" {
		if !strings.Contains(authentication, " ") {
			authentication = "Bearer " + authentication
		}
		authenticator = remoteTokenAuthenticator{}
	}
	client, err := kcclient.New(kcclient.Config{BaseURL: server, Authenticator: authenticator})
	if err != nil {
		return errorResult(err)
	}
	_, err = client.Login(ctx, kcclient.LoginRequest{
		Identity:       kcclient.Identity{Principal: principal, OnBehalfOf: FlagString(flags, "on-behalf-of")},
		Authentication: kcclient.Authentication{Authorization: authentication},
	})
	if err != nil {
		return errorResult(err)
	}
	options := kcclient.RequestOptions{RequestID: FlagString(flags, "request-id")}
	var output any
	switch path {
	case "identity whoami":
		identity, err := client.IdentityService().WhoAmI(ctx, options)
		if err != nil {
			return errorResult(err)
		}
		output = identity
	case "knowledge read":
		request := kcclient.KnowledgeReadRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"),
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
			Include: FlagStrings(flags, "include"), Exclude: FlagStrings(flags, "exclude"), Pin: remotePin(flags),
		}
		if err := client.KnowledgeService().Read(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "knowledge search":
		limit, err := remoteLimit(flags)
		if err != nil {
			return errorResult(err)
		}
		request := kcclient.KnowledgeSearchRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Query: FlagString(flags, "query"), Match: FlagStrings(flags, "match"), MatchMode: FlagString(flags, "match-mode"),
			Equal: FlagStrings(flags, "eq"), NotEqual: FlagStrings(flags, "neq"), Sort: FlagStrings(flags, "sort"),
			Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		if err := client.KnowledgeService().Search(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "knowledge relations":
		limit, err := remoteLimit(flags)
		if err != nil {
			return errorResult(err)
		}
		request := kcclient.KnowledgeRelationsRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Endpoint: FlagString(flags, "object"), RelationType: FlagString(flags, "relation-type"), Role: FlagString(flags, "role"),
			Direction: FlagString(flags, "direction"),
			Limit:     limit, Continuation: FlagString(flags, "continuation"),
		}
		if err := client.KnowledgeService().Relations(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "knowledge provenance", "knowledge log":
		request := kcclient.KnowledgeObjectRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Object: FlagString(flags, "object"),
		}
		var err error
		if path == "knowledge provenance" {
			err = client.KnowledgeService().Provenance(ctx, request, options, &output)
		} else {
			err = client.KnowledgeService().Log(ctx, request, options, &output)
		}
		if err != nil {
			return errorResult(err)
		}
	case "knowledge schema describe":
		request := kcclient.KnowledgeSchemaRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Object: FlagString(flags, "object"),
		}
		if err := client.KnowledgeService().Schema(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "knowledge binding resolve":
		request := kcclient.KnowledgeBindingRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		}
		if err := client.KnowledgeService().ResolveBinding(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "catalog show", "catalog audit", "catalog archive", "catalog repository list", "catalog workspace list", "catalog workspace show", "catalog workspace resolve", "catalog workspace check", "catalog workspace retire", "catalog repository register", "catalog repository archive":
		catalogID, err := requireRemoteFlag(flags, "catalog")
		if err != nil {
			return errorResult(err)
		}
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
		}
		if err != nil {
			return errorResult(err)
		}
	case "catalog workspace define":
		catalogID, err := requireRemoteFlag(flags, "catalog")
		if err != nil {
			return errorResult(err)
		}
		workspace, err := requireRemoteFlag(flags, "workspace")
		if err != nil {
			return errorResult(err)
		}
		revision, err := remoteIntFlag(flags, "revision")
		if err != nil {
			return errorResult(err)
		}
		sources, err := remoteWorkspaceSources(flags)
		if err != nil {
			return errorResult(err)
		}
		if err := client.CatalogService().DefineWorkspace(ctx, catalogID, kcclient.WorkspaceDefinitionRequest{Workspace: workspace, Revision: revision, Sources: sources}, options, &output); err != nil {
			return errorResult(err)
		}
	case "writer put", "writer remove", "writer commit":
		request, repository, err := remoteCommitRequest(path, flags)
		if err != nil {
			return errorResult(err)
		}
		if err := client.WriterService().Commit(ctx, repository, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "writer ingest":
		repository, err := requireRemoteFlag(flags, "repo")
		if err != nil {
			return errorResult(err)
		}
		dir, err := requireRemoteFlag(flags, "dir")
		if err != nil {
			return errorResult(err)
		}
		preview, err := writer.Ingest(dir, kernel.RepositoryID(repository), kernel.CommitID(FlagString(flags, "base")))
		if err != nil {
			return errorResult(err)
		}
		output = preview
	case "writer receipt":
		commandID, err := requireRemoteFlag(flags, "command-id")
		if err != nil {
			return errorResult(err)
		}
		if err := client.WriterService().Receipt(ctx, commandID, options, &output); err != nil {
			return errorResult(err)
		}
	case "governance proposal create":
		request, err := remoteProposalRequest(flags)
		if err != nil {
			return errorResult(err)
		}
		if err := client.GovernanceService().Proposal(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "governance preview create":
		request := kcclient.PreviewRequest{Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Proposal: FlagString(flags, "proposal")}
		if err := client.GovernanceService().Preview(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "governance preview validate":
		request := kcclient.ValidateRequest{Catalog: FlagString(flags, "catalog"), Preview: FlagString(flags, "preview")}
		if err := client.GovernanceService().Validate(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "governance validation record":
		request := kcclient.ValidationRequest{Catalog: FlagString(flags, "catalog"), Preview: FlagString(flags, "preview"), Suite: FlagString(flags, "suite"), Outcome: FlagString(flags, "outcome")}
		if err := client.GovernanceService().RecordValidation(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "governance proposal merge":
		request := kcclient.MergeRequest{Catalog: FlagString(flags, "catalog"), Proposal: FlagString(flags, "proposal"), Preview: FlagString(flags, "preview"), Validation: FlagString(flags, "validation")}
		if err := client.GovernanceService().Merge(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "admin grant add":
		request := kcclient.GrantRequest{Principal: FlagString(flags, "principal"), Actions: splitCmds(FlagString(flags, "action")), Repository: FlagString(flags, "repo"), Catalog: FlagString(flags, "catalog"), Ref: FlagString(flags, "ref"), Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Workspace: FlagString(flags, "workspace")}
		if err := client.AdminService().AddGrant(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "admin grant list":
		if err := client.AdminService().Grants(ctx, options, &output); err != nil {
			return errorResult(err)
		}
	case "admin grant remove":
		if err := client.AdminService().RemoveGrant(ctx, FlagString(flags, "id"), options, &output); err != nil {
			return errorResult(err)
		}
	case "operations projection sync":
		request := kcclient.ProjectionSyncRequest{Repository: FlagString(flags, "repo"), Commit: FlagString(flags, "commit"), Ref: FlagString(flags, "ref")}
		if err := client.OperationsService().SyncProjection(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	case "operations projection describe", "operations access describe":
		request := kcclient.ProjectionSyncRequest{Repository: FlagString(flags, "repo"), Commit: FlagString(flags, "commit"), Ref: FlagString(flags, "ref")}
		var err error
		if path == "operations projection describe" {
			err = client.OperationsService().DescribeProjection(ctx, request, options, &output)
		} else {
			err = client.OperationsService().DescribeAccessSpec(ctx, request, options, &output)
		}
		if err != nil {
			return errorResult(err)
		}
	case "operations hook add", "operations gate add":
		request := kcclient.PolicyBindingRequest{On: FlagString(flags, "on"), Phase: FlagString(flags, "phase"), Repository: FlagString(flags, "repo"), Catalog: FlagString(flags, "catalog"), Run: FlagString(flags, "run"), URL: FlagString(flags, "url"), Require: splitCmds(FlagString(flags, "require"))}
		var err error
		if path == "operations hook add" {
			err = client.OperationsService().AddHook(ctx, request, options, &output)
		} else {
			err = client.OperationsService().AddGate(ctx, request, options, &output)
		}
		if err != nil {
			return errorResult(err)
		}
	case "operations hook list", "operations gate list":
		var err error
		if path == "operations hook list" {
			err = client.OperationsService().Hooks(ctx, options, &output)
		} else {
			err = client.OperationsService().Gates(ctx, options, &output)
		}
		if err != nil {
			return errorResult(err)
		}
	case "operations hook remove", "operations gate remove":
		var err error
		if path == "operations hook remove" {
			err = client.OperationsService().RemoveHook(ctx, FlagString(flags, "id"), options, &output)
		} else {
			err = client.OperationsService().RemoveGate(ctx, FlagString(flags, "id"), options, &output)
		}
		if err != nil {
			return errorResult(err)
		}
	case "operations audit access", "operations audit hitmap":
		limit, err := remoteLimit(flags)
		if err != nil {
			return errorResult(err)
		}
		request := kcclient.AuditQueryRequest{Principal: FlagString(flags, "filter-principal"), OnBehalfOf: FlagString(flags, "filter-on-behalf-of"), Action: FlagString(flags, "action"), TraceID: FlagString(flags, "trace-id"), Repository: FlagString(flags, "repo"), Object: FlagString(flags, "object"), Limit: limit}
		if path == "operations audit access" {
			err = client.OperationsService().AccessLog(ctx, request, options, &output)
		} else {
			err = client.OperationsService().Hitmap(ctx, request, options, &output)
		}
		if err != nil {
			return errorResult(err)
		}
	case "operations audit trace":
		if err := client.OperationsService().Trace(ctx, FlagString(flags, "trace-id"), options, &output); err != nil {
			return errorResult(err)
		}
	case "operations feedback record":
		request := kcclient.FeedbackRequest{Workspace: FlagString(flags, "workspace"), TraceID: FlagString(flags, "trace-id"), Outcome: FlagString(flags, "outcome"), Message: FlagString(flags, "message")}
		if err := client.OperationsService().Feedback(ctx, request, options, &output); err != nil {
			return errorResult(err)
		}
	default:
		return errorResult(kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path))
	}
	return RunResult{Status: 0, Stdout: jsonOut(output)}
}

func requireRemoteFlag(flags map[string]FlagValue, name string) (string, error) {
	value := strings.TrimSpace(FlagString(flags, name))
	if value == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "remote command requires --%s", name)
	}
	return value, nil
}

func remoteIntFlag(flags map[string]FlagValue, name string) (int, error) {
	raw, err := requireRemoteFlag(flags, name)
	if err != nil {
		return 0, err
	}
	value, parseErr := strconv.Atoi(raw)
	if parseErr != nil {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--%s must be an integer", name)
	}
	return value, nil
}

func remoteWorkspaceSources(flags map[string]FlagValue) ([]catalog.WorkspaceSource, error) {
	if FlagString(flags, "from-repo") != "" {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote Workspace definition does not read a server repository recipe; submit explicit sources")
	}
	if file := FlagString(flags, "file"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		recipe, err := catalog.ParseWorkspaceRecipe(raw)
		if err != nil {
			return nil, err
		}
		return recipe.Sources(), nil
	}
	return workspaceSourcesFrom(FlagStrings(flags, "source"))
}

func remoteCommitRequest(path string, flags map[string]FlagValue) (kcclient.CommitRequest, string, error) {
	commandID, err := requireRemoteFlag(flags, "command-id")
	if err != nil {
		return kcclient.CommitRequest{}, "", err
	}
	if path == "writer commit" {
		file, err := requireRemoteFlag(flags, "changeset")
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		changeset, err := decodeChangeSet(raw, file)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		return kcclient.CommitRequest{CommandID: commandID, ChangeSet: changeset}, string(changeset.TargetRepository), nil
	}
	repository, err := requireRemoteFlag(flags, "repo")
	if err != nil {
		return kcclient.CommitRequest{}, "", err
	}
	var operation knowledge.Operation
	if path == "writer put" {
		value, ok, err := loadJSONFlag(flags, "--value")
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		if !ok {
			return kcclient.CommitRequest{}, "", kernel.Fail(kernel.ErrUsageInvalid, "put requires --file or --value")
		}
		operation, err = writeOperation(flags, knowledge.OpPut, value)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
	} else {
		operation, err = writeOperation(flags, knowledge.OpRemove, nil)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
	}
	change := knowledge.ChangeSet{TargetRepository: kernel.RepositoryID(repository), TargetRef: snapshotRef(flags), BaseCommit: kernel.CommitID(FlagString(flags, "base")), ExpectedTargetCommit: kernel.CommitID(FlagString(flags, "expected")), Operations: []knowledge.Operation{operation}, Message: FlagString(flags, "message"), Provenance: originFrom(flags)}
	return kcclient.CommitRequest{CommandID: commandID, ChangeSet: change}, repository, nil
}

func snapshotRef(flags map[string]FlagValue) string {
	ref := FlagString(flags, "ref")
	if ref == "" {
		return defaultRef
	}
	return ref
}

func remoteProposalRequest(flags map[string]FlagValue) (kcclient.ProposalRequest, error) {
	operations, err := proposeOperations(flags)
	if err != nil {
		return kcclient.ProposalRequest{}, err
	}
	return kcclient.ProposalRequest{Catalog: FlagString(flags, "catalog"), Repository: FlagString(flags, "repo"), ProposalID: FlagString(flags, "proposal-id"), CandidateRef: FlagString(flags, "candidate"), TargetRef: FlagString(flags, "target"), BaseCommit: FlagString(flags, "base"), Operations: operations, Message: FlagString(flags, "message"), Provenance: originFrom(flags)}, nil
}

// remoteTokenAuthenticator sends only the credential. The server derives the
// principal from its trusted authenticator and rejects caller-supplied identity
// headers; the local Identity remains useful for CLI context and audit UX.
type remoteTokenAuthenticator struct{}

func (remoteTokenAuthenticator) Login(_ context.Context, request kcclient.LoginRequest) (kcclient.Session, error) {
	return kcclient.Session{Identity: request.Identity, Authentication: request.Authentication}, nil
}

func (remoteTokenAuthenticator) Logout(context.Context, kcclient.Session) error { return nil }

func (remoteTokenAuthenticator) AuthenticateRequest(_ context.Context, session kcclient.Session, _ string, request *http.Request) error {
	request.Header.Set("Authorization", session.Authentication.Authorization)
	return nil
}

func remotePin(flags map[string]FlagValue) json.RawMessage {
	raw := strings.TrimSpace(FlagString(flags, "pin"))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		if content, err := os.ReadFile(raw); err == nil {
			raw = string(content)
		}
	}
	return json.RawMessage(raw)
}

func remoteLimit(flags map[string]FlagValue) (int, error) {
	raw := strings.TrimSpace(FlagString(flags, "limit"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--limit must be a non-negative integer")
	}
	return value, nil
}

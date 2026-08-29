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
	output, err := runRemoteRequest(ctx, client, path, flags, options)
	if err != nil {
		return errorResult(err)
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
	return kcclient.Session(request), nil
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

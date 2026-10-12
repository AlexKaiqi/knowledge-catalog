package cli

import (
	"context"
	"encoding/json"
	inv "kc/invocation"
	"net/http"
	"os"
	"strconv"
	"strings"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot/dataset"
)

func runRemoteCLI(ctx context.Context, server, path string, flags map[string]FlagValue) RunResult {
	if _, explicitHome := flags["home"]; explicitHome {
		return inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "--server and --home are mutually exclusive"))
	}
	bindRemoteEnvironment(path, flags)
	if path == "login" || path == "logout" {
		return runRemoteLogin(ctx, server, path, flags)
	}

	client, err := newRemoteSessionClient(ctx, server, flags)
	if err != nil {
		return inv.ErrorResult(err)
	}
	options := kcclient.RequestOptions{RequestID: flagString(flags, "request-id")}
	output, err := runRemoteRequest(ctx, client, path, flags, options)
	if err != nil {
		return inv.ErrorResult(err)
	}
	return RunResult{Status: 0, Stdout: jsonOut(output)}
}

func bindRemoteEnvironment(path string, flags map[string]FlagValue) {
	if strings.HasPrefix(path, "grant ") {
		return
	}
	for _, name := range []string{"repo", "pin", "source", "file", "dataset-file", "dataset", "file-source"} {
		if name == "file-source" {
			if len(flagStrings(flags, name)) > 0 {
				return
			}
			continue
		}
		if strings.TrimSpace(flagString(flags, name)) != "" {
			return
		}
	}
	if !remoteCommandInheritsKnowledgeSet(path) {
		return
	}
	if value := strings.TrimSpace(os.Getenv("KC_DATASET")); value != "" {
		flags["dataset"] = value
	}
}

func remoteCommandInheritsKnowledgeSet(path string) bool {
	switch {
	case knowledgeCLIPath(path):
		return path != "schema list"
	case path == "pin" || path == "pin check" || strings.HasPrefix(path, "dataset "):
		return true
	case path == "operations access-spec describe":
		return true
	default:
		return false
	}
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

func remoteKnowledgeSetSources(flags map[string]FlagValue) ([]dataset.KnowledgeSetSource, error) {
	if flagString(flags, "from-repo") != "" {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote Workspace definition does not read a server repository recipe; submit explicit sources")
	}
	fileEntries, err := inv.FileSourcesFrom(flagStrings(flags, "file-source"))
	if err != nil {
		return nil, err
	}
	if file := flagString(flags, "file"); file != "" {
		if len(fileEntries) > 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid,
				"--file-source combines with --source only; a recipe file carries file entries in its files section")
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		recipe, err := dataset.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return nil, err
		}
		return recipe.Sources(), nil
	}
	var sources []dataset.KnowledgeSetSource
	if items := flagStrings(flags, "source"); len(items) > 0 {
		sources, err = inv.WorkspaceSourcesFrom(items)
		if err != nil {
			return nil, err
		}
	}
	sources = append(sources, fileEntries...)
	if len(sources) == 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "at least one --source or --file-source is required")
	}
	return sources, nil
}

func remoteCommitRequest(path string, flags map[string]FlagValue) (kcclient.CommitRequest, string, error) {
	commandID, err := requireRemoteFlag(flags, "command-id")
	if err != nil {
		return kcclient.CommitRequest{}, "", err
	}
	if path == "writer commit" {
		file, err := requireRemoteFlag(flags, "changeset")
		if err != nil {
			return kcclient.CommitRequest{}, "", kernel.Fail(kernel.ErrUsageInvalid, "writer commit requires --dir or --changeset")
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		changeset, err := inv.DecodeChangeSet(raw, file)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		if flagBool(flags, "bulk") {
			changeset.BulkIngest = true
		}
		return kcclient.CommitRequest{CommandID: commandID, ChangeSet: changeset}, string(changeset.TargetRepository), nil
	}
	if flagBool(flags, "bulk") {
		return kcclient.CommitRequest{}, "", kernel.Fail(kernel.ErrUsageInvalid, "--bulk applies to writer commit, not single-operation writes")
	}
	repository, err := requireRemoteFlag(flags, "repo")
	if err != nil {
		return kcclient.CommitRequest{}, "", err
	}
	var operation knowledge.Operation
	if path == "writer put" {
		value, ok, err := inv.LoadJSONFlag(flags, "--value")
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
		if !ok {
			return kcclient.CommitRequest{}, "", kernel.Fail(kernel.ErrUsageInvalid, "put requires --file or --value")
		}
		operation, err = inv.WriteOperation(flags, knowledge.OpPut, value)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
	} else {
		operation, err = inv.WriteOperation(flags, knowledge.OpRemove, nil)
		if err != nil {
			return kcclient.CommitRequest{}, "", err
		}
	}
	change := knowledge.ChangeSet{TargetRepository: kernel.RepositoryID(repository), TargetRef: snapshotRef(flags), BaseCommit: kernel.CommitID(flagString(flags, "base")), ExpectedTargetCommit: kernel.CommitID(flagString(flags, "expected")), Operations: []knowledge.Operation{operation}, Message: flagString(flags, "message"), Provenance: inv.OriginFrom(flags)}
	return kcclient.CommitRequest{CommandID: commandID, ChangeSet: change}, repository, nil
}

func snapshotRef(flags map[string]FlagValue) string {
	ref := flagString(flags, "ref")
	if ref == "" {
		return defaultRef
	}
	return ref
}

func remoteProposalRequest(flags map[string]FlagValue) (kcclient.ProposalRequest, error) {
	operations, err := inv.ProposeOperations(flags)
	if err != nil {
		return kcclient.ProposalRequest{}, err
	}
	return kcclient.ProposalRequest{Repository: flagString(flags, "repo"), ProposalID: flagString(flags, "proposal-id"), CandidateRef: flagString(flags, "candidate"), TargetRef: flagString(flags, "target"), BaseCommit: flagString(flags, "base"), Operations: operations, Message: flagString(flags, "message"), Provenance: inv.OriginFrom(flags)}, nil
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

func discoverServerAuth(ctx context.Context, server string) (kcclient.AuthDiscovery, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := kcclient.New(kcclient.Config{BaseURL: server})
	if err != nil {
		return kcclient.AuthDiscovery{}, err
	}
	discovery, err := client.IdentityService().Discover(ctx)
	if err != nil {
		return kcclient.AuthDiscovery{}, err
	}
	if strings.TrimSpace(discovery.Mode) == "" {
		return kcclient.AuthDiscovery{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "kc server auth discovery omitted mode")
	}
	return discovery, nil
}

func bearerParts(raw string) (header, access string) {
	raw = strings.TrimSpace(raw)
	fields := strings.Fields(raw)
	if len(fields) == 2 && strings.EqualFold(fields[0], "bearer") {
		return "Bearer " + fields[1], fields[1]
	}
	return "Bearer " + raw, raw
}

// runRemoteLogin handles login/logout against a remote server without
// requiring a pre-existing principal. Login only changes the client
// credential store (Taihu, token, or local principal).
func runRemoteLogin(ctx context.Context, server, path string, flags map[string]FlagValue) RunResult {
	cx := &invocation{Command: path, Home: "", Flags: flags, Context: ctx}
	var result any
	var err error
	if path == "login" {
		result, err = verbLogin(cx)
	} else {
		result, err = verbLogout(cx)
	}
	if err != nil {
		return inv.ErrorResult(err)
	}
	return RunResult{Status: 0, Stdout: jsonOut(result)}
}

func remotePin(flags map[string]FlagValue) json.RawMessage {
	raw := strings.TrimSpace(flagString(flags, "pin"))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		if content, err := os.ReadFile(raw); err == nil {
			raw = string(content)
		}
	}
	var saved taskKnowledgeSetPin
	if dataset.DecodeJSON([]byte(raw), &saved) == nil && saved.Definition != nil {
		flags[workspaceDefinitionFlag] = saved.Definition
		if pin, err := json.Marshal(saved.ResolvedKnowledgeSet); err == nil {
			return pin
		}
	}
	return json.RawMessage(raw)
}

func remotePinDocument(flags map[string]FlagValue) json.RawMessage {
	raw := strings.TrimSpace(flagString(flags, "pin"))
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
	raw := strings.TrimSpace(flagString(flags, "limit"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--limit must be a non-negative integer")
	}
	return value, nil
}

// remoteNonNegativeInt parses one non-negative integer flag. optional=false
// makes an absent value a usage error (for example traverse --max-hops).
func remoteNonNegativeInt(flags map[string]FlagValue, name string, optional bool) (int, error) {
	raw := strings.TrimSpace(flagString(flags, name))
	if raw == "" {
		if optional {
			return 0, nil
		}
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--%s is required", name)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--%s must be a non-negative integer", name)
	}
	return value, nil
}

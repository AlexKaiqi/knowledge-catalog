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
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func runRemoteCLI(ctx context.Context, server, path string, flags map[string]FlagValue) RunResult {
	if _, explicitHome := flags["home"]; explicitHome {
		return inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "--server and --home are mutually exclusive"))
	}
	bindRemoteEnvironment(path, flags)
	if err := preflightObjectClient(path, flags); err != nil {
		return inv.ErrorResult(err)
	}
	if err := preflightSnapshotClient(path, flags); err != nil {
		return inv.ErrorResult(err)
	}
	if err := preflightDiscoveryClient(path, flags); err != nil {
		return inv.ErrorResult(err)
	}
	if err := preflightDeclarativeIndexClient(path, flags); err != nil {
		return inv.ErrorResult(err)
	}
	if err := preflightLiveAccessClient(path, flags); err != nil {
		return inv.ErrorResult(err)
	}
	if path == "login" || path == "logout" {
		return runRemoteLogin(ctx, server, path, flags)
	}

	client, err := newRemoteSessionClient(ctx, server, flags)
	if err != nil {
		return inv.ErrorResult(err)
	}
	options := kcclient.RequestOptions{RequestID: invoke.FlagString(flags, "request-id")}
	output, err := runRemoteRequest(ctx, client, path, flags, options)
	if err != nil {
		return inv.ErrorResult(err)
	}
	return RunResult{Status: 0, Stdout: inv.JSONOut(output)}
}

func bindRemoteEnvironment(path string, flags map[string]FlagValue) {
	if objectCommandPath(path) {
		return
	}
	if strings.HasPrefix(path, "grant ") {
		return
	}
	for _, name := range []string{"repo", "pin", "source", "file", "dataset-file", "dataset", "file-source"} {
		if name == "file-source" {
			if len(invoke.FlagStrings(flags, name)) > 0 {
				return
			}
			continue
		}
		if strings.TrimSpace(invoke.FlagString(flags, name)) != "" {
			return
		}
	}
	if !remoteCommandInheritsDataset(path) {
		return
	}
	if value := strings.TrimSpace(os.Getenv("KC_DATASET")); value != "" {
		flags["dataset"] = value
	}
}

func remoteCommandInheritsDataset(path string) bool {
	if path == "plugin" {
		return false
	}
	switch {
	case objectCLIPath(path):
		return true
	case strings.HasPrefix(path, "dataset ") || strings.HasPrefix(path, "files "):
		return true
	case path == "operations access-spec describe":
		return true
	default:
		return false
	}
}

func remoteCommitRequest(path string, flags map[string]FlagValue) (kcclient.CommitRequest, string, error) {
	commandID, err := inv.RequireRemoteFlag(flags, "command-id")
	if err != nil {
		return kcclient.CommitRequest{}, "", err
	}
	if path == "writer commit" {
		file, err := inv.RequireRemoteFlag(flags, "changeset")
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
		if invoke.FlagBool(flags, "bulk") {
			changeset.BulkIngest = true
		}
		return kcclient.CommitRequest{CommandID: commandID, ChangeSet: changeset}, string(changeset.TargetRepository), nil
	}
	return kcclient.CommitRequest{}, "", kernel.Fail(kernel.ErrUsageInvalid, "expected writer commit")
}

func snapshotRef(flags map[string]FlagValue) string {
	ref := invoke.FlagString(flags, "ref")
	if ref == "" {
		return snapshot.DefaultRef
	}
	return ref
}

func remoteProposalRequest(flags map[string]FlagValue) (kcclient.ProposalRequest, error) {
	operations, err := inv.ProposeOperations(flags)
	if err != nil {
		return kcclient.ProposalRequest{}, err
	}
	return kcclient.ProposalRequest{Repository: invoke.FlagString(flags, "repo"), ProposalID: invoke.FlagString(flags, "proposal-id"), CandidateRef: invoke.FlagString(flags, "candidate"), TargetRef: invoke.FlagString(flags, "target"), BaseCommit: invoke.FlagString(flags, "base"), Operations: operations, Message: invoke.FlagString(flags, "message"), Provenance: inv.OriginFrom(flags)}, nil
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
	cx := &inv.Invocation{Command: path, Home: "", Flags: flags, Context: ctx}
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
	return RunResult{Status: 0, Stdout: inv.JSONOut(result)}
}

func remotePin(flags map[string]FlagValue) json.RawMessage {
	raw := strings.TrimSpace(invoke.FlagString(flags, "pin"))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		if content, err := os.ReadFile(raw); err == nil {
			raw = string(content)
		}
	}
	var saved inv.TaskDatasetPin
	if dataset.DecodeJSON([]byte(raw), &saved) == nil && saved.Definition != nil {
		flags[inv.WorkspaceDefinitionFlag] = saved.Definition
		if pin, err := json.Marshal(saved.ResolvedDataset); err == nil {
			return pin
		}
	}
	return json.RawMessage(raw)
}

func remotePinDocument(flags map[string]FlagValue) json.RawMessage {
	raw := strings.TrimSpace(invoke.FlagString(flags, "pin"))
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
	raw := strings.TrimSpace(invoke.FlagString(flags, "limit"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--limit must be a non-negative integer")
	}
	return value, nil
}

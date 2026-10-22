package cli

import (
	"context"
	"encoding/json"
	clientsession "kc/client/session"
	inv "kc/invocation"
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

	config, err := clientsession.Configure(ctx, server, clientsession.Selection{Principal: invoke.FlagString(flags, "as"), OnBehalfOf: invoke.FlagString(flags, "on-behalf-of")})
	if err != nil {
		return inv.ErrorResult(err)
	}
	client, err := kcclient.New(config)
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

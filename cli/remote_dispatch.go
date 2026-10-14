package cli

import (
	"context"
	snapshotclient "kc/snapshot/client"
	"strings"

	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteRequest(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if objectCommandPath(path) {
		return executeObjectCommand(ctx, client, path, flags, options)
	}
	if snapshotCommandPath(path) {
		return executeSnapshotCommand(ctx, client, path, flags, options)
	}
	if discoveryCommandPath(path) {
		return executeDiscoveryCommand(ctx, client, path, flags, options)
	}
	if declarativeIndexCommandPath(path) {
		return executeDeclarativeIndexCommand(ctx, client, path, flags, options)
	}
	if liveAccessCommandPath(path) {
		return executeLiveAccessCommand(ctx, client, path, flags, options)
	}
	switch {
	case path == "admission show":
		return runRemoteAdmissionSharing(ctx, client, path, flags, options)
	case path == "whoami":
		return client.IdentityService().WhoAmI(ctx, options)
	case path == "create":
		return runRemoteRepositoryCreate(ctx, client, flags, options)
	case path == "diff":
		return runRemoteDesiredDiff(ctx, client, flags, options)
	case strings.HasPrefix(path, "writer "):
		return runRemoteWriter(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "operations "):
		return runRemoteOperations(ctx, client, path, flags, options)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteAdmissionSharing(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var out any
	if path == "admission show" {
		err := client.AdminService().Admission(ctx, snapshotclient.RequestOptions{RequestID: options.RequestID}, &out)
		return out, err
	}
	return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown admission operation")
}

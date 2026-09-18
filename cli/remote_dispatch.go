package cli

import (
	"context"
	"strings"

	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteRequest(ctx context.Context, client *kcclient.Client, server, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	switch {
	case path == "admission show":
		return runRemoteAdmissionSharing(ctx, client, path, flags, options)
	case path == "whoami":
		return client.IdentityService().WhoAmI(ctx, options)
	case path == "show" || path == "attach" || path == "create" || path == "detach" || strings.HasPrefix(path, "catalog "):
		return runRemoteCatalog(ctx, client, server, path, flags, options)
	case knowledgeCLIPath(path):
		return runRemoteKnowledge(ctx, client, path, flags, options)
	case path == "diff":
		return runRemoteDesiredDiff(ctx, client, flags, options)
	case path == "pin" || path == "pin check" || strings.HasPrefix(path, "dataset ") || strings.HasPrefix(path, "workspace "):
		return runRemoteWorkspace(ctx, client, server, path, flags, options)
	case strings.HasPrefix(path, "writer "):
		return runRemoteWriter(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "governance "):
		return runRemoteGovernance(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "grant "):
		return runRemoteAdmin(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "operations "):
		return runRemoteOperations(ctx, client, path, flags, options)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

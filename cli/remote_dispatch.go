package cli

import (
	"context"
	"strings"

	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteRequest(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	switch {
	case path == "identity whoami":
		return client.IdentityService().WhoAmI(ctx, options)
	case strings.HasPrefix(path, "knowledge "):
		return runRemoteKnowledge(ctx, client, path, flags, options)
	case path == "resource access":
		return runRemoteResourceAccess(ctx, client, flags, options)
	case strings.HasPrefix(path, "catalog "):
		return runRemoteCatalog(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "writer "):
		return runRemoteWriter(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "governance "):
		return runRemoteGovernance(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "admin "):
		return runRemoteAdmin(ctx, client, path, flags, options)
	case strings.HasPrefix(path, "operations "):
		return runRemoteOperations(ctx, client, path, flags, options)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

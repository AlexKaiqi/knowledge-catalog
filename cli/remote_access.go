package cli

import (
	"context"

	kcclient "kc/client"
	inv "kc/invocation"
	"kc/kernel"
)

func runRemoteAdmin(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	service := client.AdminService()
	switch path {
	case "grant add", "admin grant add":
		request := kcclient.GrantRequest{Principal: flagString(flags, "principal"), Actions: splitCmds(flagString(flags, "action")), Repository: flagString(flags, "repo"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Dataset: flagString(flags, "dataset")}
		err := service.AddGrant(ctx, request, options, &output)
		return output, err
	case "grant list", "admin grant list":
		return runRemoteGrantList(ctx, client, flags, options)
	case "grant remove", "admin grant remove":
		err := service.RemoveGrant(ctx, flagString(flags, "id"), options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteGrantList(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	action := flagString(flags, "action")
	principal := flagString(flags, "principal")
	filter := kcclient.GrantRequest{Principal: principal, Repository: flagString(flags, "repo"), Dataset: flagString(flags, "dataset"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect")}
	if action != "" {
		if principal == "" {
			identity, ok, err := client.Identity(ctx)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, kernel.Fail(kernel.ErrUnauthenticated, "grant question requires an authenticated principal")
			}
			principal = identity.Principal
		}
		// Keep the CLI's existing explicit-rule question, including global rules.
		// It consequently requires a global grant inventory read capability.
		filter = kcclient.GrantRequest{Principal: principal, Actions: []string{action}}
	}
	var listed inv.GrantList
	if err := client.AdminService().GrantsMatching(ctx, filter, options, &listed); err != nil {
		return nil, err
	}
	if action == "" {
		return listed, nil
	}
	return inv.EvaluateAllow(listed.Rules, inv.AllowQuery{Principal: principal, Action: action, Repo: flagString(flags, "repo"), Dataset: flagString(flags, "dataset"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect")})
}

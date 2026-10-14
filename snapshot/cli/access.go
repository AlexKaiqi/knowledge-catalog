package cli

import (
	"context"

	"kc/kernel"
	"kc/snapshot/access"
	kcclient "kc/snapshot/client"
)

func (e *executor) grants(ctx context.Context, path string, flags map[string]any, options kcclient.RequestOptions) (any, error) {
	var output any
	service := e.client.AdminService()
	switch path {
	case "grant add":
		request := kcclient.GrantRequest{Principal: flagString(flags, "principal"), Actions: splitCmds(flagString(flags, "action")), Repository: flagString(flags, "repo"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Dataset: flagString(flags, "dataset")}
		err := service.AddGrant(ctx, request, options, &output)
		return output, err
	case "grant list":
		return runRemoteGrantList(ctx, e.client, flags, options, e.context.Principal)
	case "grant update":
		id, request, err := grantUpdateRequestFromFlags(flags)
		if err != nil {
			return nil, err
		}
		err = service.UpdateGrant(ctx, id, request, options, &output)
		return output, err
	case "grant remove":
		err := service.RemoveGrant(ctx, flagString(flags, "id"), options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

func runRemoteGrantList(ctx context.Context, client kcclient.Client, flags map[string]any, options kcclient.RequestOptions, currentPrincipal string) (any, error) {
	action := flagString(flags, "action")
	principal := flagString(flags, "principal")
	filter := kcclient.GrantRequest{Principal: principal, Repository: flagString(flags, "repo"), Dataset: flagString(flags, "dataset"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect")}
	if action != "" {
		if principal == "" {
			principal = currentPrincipal
			if principal == "" {
				return nil, kernel.Fail(kernel.ErrUnauthenticated, "grant question requires an authenticated principal")
			}
		}
		// Keep the CLI's existing explicit-rule question, including global rules.
		// It consequently requires a global grant inventory read capability.
		filter = kcclient.GrantRequest{Principal: principal, Actions: []string{action}}
	}
	var listed access.GrantList
	if err := client.AdminService().GrantsMatching(ctx, filter, options, &listed); err != nil {
		return nil, err
	}
	if action == "" {
		// The CLI returns explicit rules only, and keeps its exact repository
		// filter even when a deployed compatibility response contains a ledger.
		return access.Inventory(listed.Rules, access.Query{Repo: flagString(flags, "repo")}), nil
	}
	match := access.Match
	if action == access.RepositoryDatasetPublish {
		match = access.MatchSourcePublication
	}
	rule, allowed := match(listed.Rules, access.Query{Principal: principal, Action: action, Repo: flagString(flags, "repo"), Dataset: flagString(flags, "dataset"), Ref: flagString(flags, "ref"), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect")})
	if !allowed {
		return nil, kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", principal, action)
	}
	return map[string]any{"allow": true, "ruleId": rule.ID}, nil
}

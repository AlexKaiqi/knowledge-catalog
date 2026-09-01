package cli

import (
	"context"
	"encoding/json"

	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteResourceAccess(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	request := kcclient.KnowledgeResourceAccessRequest{
		Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
		Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		Operation: FlagString(flags, "operation"),
	}
	if raw := FlagString(flags, "input"); raw != "" {
		request.Input = json.RawMessage(raw)
	}
	var output any
	err := client.KnowledgeService().AccessResource(ctx, request, options, &output)
	return output, err
}

func runRemoteKnowledge(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var output any
	service := client.KnowledgeService()
	switch path {
	case "knowledge read":
		request := kcclient.KnowledgeReadRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"),
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
			Include: FlagStrings(flags, "include"), Exclude: FlagStrings(flags, "exclude"), Pin: remotePin(flags),
		}
		err := service.Read(ctx, request, options, &output)
		return output, err
	case "knowledge search":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.KnowledgeSearchRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Query: FlagString(flags, "query"), Match: FlagStrings(flags, "match"), MatchMode: FlagString(flags, "match-mode"),
			Equal: FlagStrings(flags, "eq"), NotEqual: FlagStrings(flags, "neq"), In: FlagStrings(flags, "in"),
			Exists: FlagStrings(flags, "exists"), Missing: FlagStrings(flags, "missing"), Prefix: FlagStrings(flags, "prefix"),
			GreaterThan: FlagStrings(flags, "gt"), GreaterEqual: FlagStrings(flags, "gte"),
			LessThan: FlagStrings(flags, "lt"), LessEqual: FlagStrings(flags, "lte"), Sort: FlagStrings(flags, "sort"),
			Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		err = service.Search(ctx, request, options, &output)
		return output, err
	case "knowledge relations":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.KnowledgeRelationsRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Endpoint: FlagString(flags, "object"), RelationType: FlagString(flags, "relation-type"), Role: FlagString(flags, "role"),
			Direction: FlagString(flags, "direction"), Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		err = service.Relations(ctx, request, options, &output)
		return output, err
	case "knowledge provenance", "knowledge log":
		request := kcclient.KnowledgeObjectRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Object: FlagString(flags, "object"),
		}
		if path == "knowledge provenance" {
			err := service.Provenance(ctx, request, options, &output)
			return output, err
		}
		err := service.Log(ctx, request, options, &output)
		return output, err
	case "knowledge schema describe":
		request := kcclient.KnowledgeSchemaRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Object: FlagString(flags, "object"),
		}
		err := service.Schema(ctx, request, options, &output)
		return output, err
	case "knowledge schema browse":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		// Discovery is pinned to one explicit Repository basis, not a Workspace:
		// a consumer may browse Schemas before choosing a knowledge set.
		request := kcclient.KnowledgeSchemaPageRequest{
			Repository: FlagString(flags, "repo"), Commit: FlagString(flags, "commit"),
			Ref: FlagString(flags, "ref"), Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		err = service.BrowseSchemas(ctx, request, options, &output)
		return output, err
	case "knowledge binding resolve":
		request := kcclient.KnowledgeBindingRequest{
			Catalog: FlagString(flags, "catalog"), Workspace: FlagString(flags, "workspace"), Pin: remotePin(flags),
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		}
		err := service.ResolveBinding(ctx, request, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

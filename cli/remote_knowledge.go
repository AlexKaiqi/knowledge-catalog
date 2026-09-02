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
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
			Include: FlagStrings(flags, "include"), Exclude: FlagStrings(flags, "exclude"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Workspace, &request.Pin, &request.Repository, &request.Commit, &request.Ref)
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
			Endpoint: FlagString(flags, "object"), RelationType: FlagString(flags, "relation-type"), Role: FlagString(flags, "role"),
			Direction: FlagString(flags, "direction"), Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Workspace, &request.Pin, &request.Repository, &request.Commit, &request.Ref)
		err = service.Relations(ctx, request, options, &output)
		return output, err
	case "knowledge provenance", "knowledge log":
		request := kcclient.KnowledgeObjectRequest{Object: FlagString(flags, "object")}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Workspace, &request.Pin, &request.Repository, &request.Commit, &request.Ref)
		if path == "knowledge provenance" {
			err := service.Provenance(ctx, request, options, &output)
			return output, err
		}
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request.Limit = limit
		request.Continuation = FlagString(flags, "continuation")
		err = service.Log(ctx, request, options, &output)
		return output, err
	case "knowledge resolve":
		request := kcclient.KnowledgeResolveRequest{
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Workspace, &request.Pin, &request.Repository, &request.Commit, &request.Ref)
		err := service.Resolve(ctx, request, options, &output)
		return output, err
	case "knowledge schema describe":
		request := kcclient.KnowledgeSchemaRequest{Object: FlagString(flags, "object")}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Workspace, &request.Pin, &request.Repository, &request.Commit, &request.Ref)
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
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Workspace, &request.Pin, &request.Repository, &request.Commit, &request.Ref)
		err := service.ResolveBinding(ctx, request, options, &output)
		return output, err
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

// applyRemoteKnowledgeBasis fills either a maintainer Repository pin or a
// consumer Workspace pin. A Repository target never carries Workspace/pin, so
// an inherited consumer context cannot force the provider read-back path onto
// a knowledge set.
func applyRemoteKnowledgeBasis(flags map[string]FlagValue, catalog, workspace *string, pin *json.RawMessage, repository, commit, ref *string) {
	*catalog = FlagString(flags, "catalog")
	if repo := FlagString(flags, "repo"); repo != "" {
		*repository = repo
		*commit = FlagString(flags, "commit")
		*ref = FlagString(flags, "ref")
		return
	}
	*workspace = FlagString(flags, "workspace")
	*pin = remotePin(flags)
}

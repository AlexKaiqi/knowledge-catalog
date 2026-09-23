package cli

import (
	"context"
	"encoding/json"

	"kc/catalog"
	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteResourceAccess(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	request := kcclient.KnowledgeResourceAccessRequest{
		Object: FlagString(flags, "object"),
	}
	applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
	if path == "invoke" {
		request.Operation = FlagString(flags, "operation")
		if raw := FlagString(flags, "input"); raw != "" {
			request.Input = json.RawMessage(raw)
		}
	} else {
		request.Aspect = FlagString(flags, "aspect")
		request.Member = FlagString(flags, "member")
	}
	var output any
	err := client.KnowledgeService().AccessResource(ctx, request, options, &output)
	return output, err
}

func runRemoteKnowledge(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	discovery := catalogSearchRequested(path, flags)
	if err := prepareKnowledgePinContext(flags); err != nil {
		return nil, err
	}
	var output any
	service := client.KnowledgeService()
	switch path {
	case "read":
		request := kcclient.KnowledgeReadRequest{
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
			Include: FlagStrings(flags, "include"), Exclude: FlagStrings(flags, "exclude"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err := service.Read(ctx, request, options, &output)
		return output, err
	case "search":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.KnowledgeSearchRequest{
			Query: FlagString(flags, "query"), Match: FlagStrings(flags, "match"), MatchMode: FlagString(flags, "match-mode"),
			Equal: FlagStrings(flags, "eq"), NotEqual: FlagStrings(flags, "neq"), In: FlagStrings(flags, "in"),
			Exists: FlagStrings(flags, "exists"), Missing: FlagStrings(flags, "missing"), Prefix: FlagStrings(flags, "prefix"),
			Contains:    FlagStrings(flags, "contains"),
			GreaterThan: FlagStrings(flags, "gt"), GreaterEqual: FlagStrings(flags, "gte"),
			LessThan: FlagStrings(flags, "lt"), LessEqual: FlagStrings(flags, "lte"), Sort: FlagStrings(flags, "sort"),
			Limit: limit, Continuation: FlagString(flags, "continuation"),
			Recall: FlagString(flags, "recall"),
		}
		if discovery {
			if err := prepareRemoteCatalogDiscovery(ctx, client, flags, options); err != nil {
				return nil, err
			}
			request.CatalogDiscovery = true
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err = service.Search(ctx, request, options, &output)
		return output, err
	case "relations":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.KnowledgeRelationsRequest{
			Endpoint: FlagString(flags, "object"), RelationType: FlagString(flags, "relation-type"), Role: FlagString(flags, "role"),
			Direction: FlagString(flags, "direction"), Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err = service.Relations(ctx, request, options, &output)
		return output, err
	case "traverse":
		maxHops, err := remoteNonNegativeInt(flags, "max-hops", false)
		if err != nil {
			return nil, err
		}
		minHops, err := remoteNonNegativeInt(flags, "min-hops", true)
		if err != nil {
			return nil, err
		}
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		request := kcclient.KnowledgeTraverseRequest{
			Endpoint: FlagString(flags, "object"), RelationType: FlagString(flags, "relation-type"), Role: FlagString(flags, "role"),
			Direction: FlagString(flags, "direction"), MinHops: minHops, MaxHops: maxHops,
			Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err = service.Traverse(ctx, request, options, &output)
		return output, err
	case "provenance", "log":
		if usesAddress(flags) {
			if path == "log" {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "log is object history; do not pass --aspect or --member")
			}
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "provenance is object-level; do not pass --aspect or --member")
		}
		request := kcclient.KnowledgeObjectRequest{Object: FlagString(flags, "object")}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		if path == "provenance" {
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
	case "resolve":
		request := kcclient.KnowledgeResolveRequest{
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err := service.Resolve(ctx, request, options, &output)
		return output, err
	case "schema describe":
		request := kcclient.KnowledgeSchemaRequest{Object: FlagString(flags, "object")}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err := service.Schema(ctx, request, options, &output)
		return output, err
	case "schema list":
		limit, err := remoteLimit(flags)
		if err != nil {
			return nil, err
		}
		// Discovery is pinned to one explicit Repository basis, not a Workspace:
		// a consumer may browse Schemas before choosing a knowledge set.
		request := kcclient.KnowledgeSchemaPageRequest{
			KnowledgeRepoPin: kcclient.KnowledgeRepoPin{
				Repository: FlagString(flags, "repo"), Commit: FlagString(flags, "commit"), Ref: FlagString(flags, "ref"),
			},
			Limit: limit, Continuation: FlagString(flags, "continuation"),
		}
		err = service.BrowseSchemas(ctx, request, options, &output)
		return output, err
	case "binding show":
		request := kcclient.KnowledgeBindingRequest{
			Object: FlagString(flags, "object"), Aspect: FlagString(flags, "aspect"), Member: FlagString(flags, "member"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err := service.ResolveBinding(ctx, request, options, &output)
		return output, err
	case "access", "invoke":
		return runRemoteResourceAccess(ctx, client, path, flags, options)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
}

// applyRemoteKnowledgeBasis fills either a maintainer Repository pin or a
// consumer Workspace pin. A Repository target never carries Workspace/pin, so
// an inherited consumer context cannot force the provider read-back path onto
// a knowledge set.
func applyRemoteKnowledgeBasis(flags map[string]FlagValue, catalog, workspace *string, pin *json.RawMessage, repository, commit, ref *string, definition **catalog.KnowledgeSet) {
	*catalog = FlagString(flags, "catalog")
	if repo := FlagString(flags, "repo"); repo != "" {
		*repository = repo
		*commit = FlagString(flags, "commit")
		*ref = FlagString(flags, "ref")
		return
	}
	*workspace = FlagString(flags, "dataset")
	*pin = remotePin(flags)
	*definition = suppliedKnowledgeSet(flags)
}

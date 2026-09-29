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
		Object: flagString(flags, "object"),
	}
	applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
	if path == "invoke" {
		request.Operation = flagString(flags, "operation")
		if raw := flagString(flags, "input"); raw != "" {
			request.Input = json.RawMessage(raw)
		}
	} else {
		request.Aspect = flagString(flags, "aspect")
		request.Member = flagString(flags, "member")
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
			Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Member: flagString(flags, "member"),
			Include: flagStrings(flags, "include"), Exclude: flagStrings(flags, "exclude"),
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
			Query: flagString(flags, "query"), Match: flagStrings(flags, "match"), MatchMode: flagString(flags, "match-mode"),
			Equal: flagStrings(flags, "eq"), NotEqual: flagStrings(flags, "neq"), In: flagStrings(flags, "in"),
			Exists: flagStrings(flags, "exists"), Missing: flagStrings(flags, "missing"), Prefix: flagStrings(flags, "prefix"),
			Contains:    flagStrings(flags, "contains"),
			GreaterThan: flagStrings(flags, "gt"), GreaterEqual: flagStrings(flags, "gte"),
			LessThan: flagStrings(flags, "lt"), LessEqual: flagStrings(flags, "lte"), Sort: flagStrings(flags, "sort"),
			Limit: limit, Continuation: flagString(flags, "continuation"),
			Recall: flagString(flags, "recall"),
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
			Endpoint: flagString(flags, "object"), RelationType: flagString(flags, "relation-type"), Role: flagString(flags, "role"),
			Direction: flagString(flags, "direction"), Limit: limit, Continuation: flagString(flags, "continuation"),
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
			Endpoint: flagString(flags, "object"), RelationType: flagString(flags, "relation-type"), Role: flagString(flags, "role"),
			Direction: flagString(flags, "direction"), MinHops: minHops, MaxHops: maxHops,
			Limit: limit, Continuation: flagString(flags, "continuation"),
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
		request := kcclient.KnowledgeObjectRequest{Object: flagString(flags, "object")}
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
		request.Continuation = flagString(flags, "continuation")
		err = service.Log(ctx, request, options, &output)
		return output, err
	case "resolve":
		request := kcclient.KnowledgeResolveRequest{
			Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Member: flagString(flags, "member"),
		}
		applyRemoteKnowledgeBasis(flags, &request.Catalog, &request.Dataset, &request.Pin, &request.Repository, &request.Commit, &request.Ref, &request.Definition)
		err := service.Resolve(ctx, request, options, &output)
		return output, err
	case "schema describe":
		request := kcclient.KnowledgeSchemaRequest{Object: flagString(flags, "object")}
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
				Repository: flagString(flags, "repo"), Commit: flagString(flags, "commit"), Ref: flagString(flags, "ref"),
			},
			Limit: limit, Continuation: flagString(flags, "continuation"),
		}
		err = service.BrowseSchemas(ctx, request, options, &output)
		return output, err
	case "binding show":
		request := kcclient.KnowledgeBindingRequest{
			Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Member: flagString(flags, "member"),
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
	*catalog = flagString(flags, "catalog")
	if repo := flagString(flags, "repo"); repo != "" {
		*repository = repo
		*commit = flagString(flags, "commit")
		*ref = flagString(flags, "ref")
		return
	}
	*workspace = flagString(flags, "dataset")
	*pin = remotePin(flags)
	*definition = suppliedKnowledgeSet(flags)
}

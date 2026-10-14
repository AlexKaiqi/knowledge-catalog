package cli

import (
	"context"
	"kc/discovery/client"
	"kc/kernel"
)

type executor struct {
	client  client.Client
	context Context
}

func New(remote client.Client, caller Context) Executor {
	return &executor{client: remote, context: caller}
}
func (e *executor) scope(flags map[string]any) client.ObjectScope {
	scope := client.ObjectScope{Repository: flagString(flags, "repo"), Commit: flagString(flags, "commit"), Ref: flagString(flags, "ref")}
	if scope.Repository != "" {
		return scope
	}
	scope.Dataset = flagString(flags, "dataset")
	if scope.Dataset == "" {
		scope.Dataset = e.context.Dataset
	}
	scope.Pin, scope.Definition = e.context.Pin, e.context.Definition
	return scope
}
func (e *executor) Execute(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	if _, ok := command(path); !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Discovery CLI does not implement %s", path)
	}
	if err := e.Validate(path, flags); err != nil {
		return nil, err
	}
	if flags["help"] == true {
		help := leafHelpEntries()[path]
		return help.Description + "\n\n" + help.Usage, nil
	}
	if e.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Discovery CLI requires an authenticated HTTP client")
	}
	var output any
	var err error
	if path == "plugin" {
		err = e.client.PluginHelp(ctx, flagString(flags, "name"), options, &output)
		return output, err
	}
	scope := e.scope(flags)
	limit, err := integer(flags, "limit", true)
	if err != nil {
		return nil, err
	}
	switch path {
	case "search":
		request := client.SearchRequest{
			ObjectScope: scope, InventoryDiscovery: e.context.InventoryDiscovery && scope.Repository == "",
			Query: flagString(flags, "query"), Match: flagStrings(flags, "match"), MatchMode: flagString(flags, "match-mode"),
			Equal: flagStrings(flags, "eq"), NotEqual: flagStrings(flags, "neq"), In: flagStrings(flags, "in"),
			Exists: flagStrings(flags, "exists"), Missing: flagStrings(flags, "missing"), Prefix: flagStrings(flags, "prefix"), Contains: flagStrings(flags, "contains"),
			GreaterThan: flagStrings(flags, "gt"), GreaterEqual: flagStrings(flags, "gte"), LessThan: flagStrings(flags, "lt"), LessEqual: flagStrings(flags, "lte"), Sort: flagStrings(flags, "sort"),
			Limit: limit, Continuation: flagString(flags, "continuation"), Recall: flagString(flags, "recall"),
		}
		err = e.client.Search(ctx, request, options, &output)
	case "relations":
		err = e.client.Relations(ctx, client.RelationsRequest{ObjectScope: scope, Endpoint: flagString(flags, "object"), RelationType: flagString(flags, "relation-type"), Role: flagString(flags, "role"), Direction: flagString(flags, "direction"), Limit: limit, Continuation: flagString(flags, "continuation")}, options, &output)
	case "traverse":
		max, _ := integer(flags, "max-hops", false)
		min, _ := integer(flags, "min-hops", true)
		err = e.client.Traverse(ctx, client.TraverseRequest{ObjectScope: scope, Endpoint: flagString(flags, "object"), RelationType: flagString(flags, "relation-type"), Role: flagString(flags, "role"), Direction: flagString(flags, "direction"), MinHops: min, MaxHops: max, Limit: limit, Continuation: flagString(flags, "continuation")}, options, &output)
	case "operations access-spec describe":
		err = e.client.DescribeAccessSpec(ctx, client.AccessSpecDescribeRequest{Dataset: scope.Dataset, Repository: scope.Repository, Pin: scope.Pin}, options, &output)
	}
	return output, err
}

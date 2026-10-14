package cli

import (
	"context"
	"kc/kernel"
	"kc/object/client"
)

type executor struct {
	client  client.Client
	context Context
}

func New(remote client.Client, caller Context) Executor {
	return &executor{client: remote, context: caller}
}

func (e *executor) Execute(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	if _, ok := command(path); !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Object CLI does not implement %s", path)
	}
	if err := e.Validate(path, flags); err != nil {
		return nil, err
	}
	if flags["help"] == true {
		help := leafHelp[path]
		return help.Description + "\n\n" + help.Usage, nil
	}
	if e.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Object CLI requires an authenticated HTTP client")
	}
	if path == "writer put" || path == "writer remove" {
		return e.write(ctx, path, flags, options)
	}
	return e.read(ctx, path, flags, options)
}

func (e *executor) scope(flags map[string]any) client.KnowledgeScope {
	scope := client.KnowledgeScope{Repository: flagString(flags, "repo"), Commit: flagString(flags, "commit"), Ref: flagString(flags, "ref")}
	if scope.Repository == "" && flagString(flags, "reference") == "" {
		scope.Dataset = flagString(flags, "dataset")
		if scope.Dataset == "" {
			scope.Dataset = e.context.Dataset
		}
	} else {
		scope.Dataset = flagString(flags, "dataset")
	}
	return scope
}

func (e *executor) read(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	var output any
	var err error
	scope := e.scope(flags)
	switch path {
	case "read":
		err = e.client.Read(ctx, client.ReadRequest{KnowledgeDatasetIdentity: client.KnowledgeDatasetIdentity{Dataset: flagString(flags, "dataset")}, Reference: flagString(flags, "reference"), Commit: flagString(flags, "commit"), Ref: flagString(flags, "ref")}, options, &output)
	case "resolve":
		err = e.client.Resolve(ctx, client.ResolveRequest{KnowledgeScope: scope, Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Member: flagString(flags, "member")}, options, &output)
	case "provenance":
		err = e.client.Provenance(ctx, client.ObjectRequest{KnowledgeScope: scope, Object: flagString(flags, "object")}, options, &output)
	case "log":
		limit, _ := limitFrom(flags)
		err = e.client.Log(ctx, client.ObjectRequest{KnowledgeScope: scope, Object: flagString(flags, "object"), Limit: limit, Continuation: flagString(flags, "continuation")}, options, &output)
	case "schema describe":
		err = e.client.Schema(ctx, client.SchemaRequest{KnowledgeScope: scope, Object: flagString(flags, "object")}, options, &output)
	case "schema list":
		limit, _ := limitFrom(flags)
		err = e.client.BrowseSchemas(ctx, client.SchemaPageRequest{KnowledgeRepoPin: client.KnowledgeRepoPin{Repository: scope.Repository, Commit: scope.Commit, Ref: scope.Ref}, Limit: limit, Continuation: flagString(flags, "continuation")}, options, &output)
	}
	return output, err
}

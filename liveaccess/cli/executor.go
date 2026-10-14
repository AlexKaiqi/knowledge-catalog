package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"kc/kernel"
	objectclient "kc/object/client"
)

type executor struct {
	client  Client
	context Context
}

func New(remote Client, caller Context) Executor {
	return &executor{client: remote, context: caller}
}

func flagString(flags map[string]any, name string) string {
	if flags[name] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(flags[name]))
}

func (e *executor) scope(flags map[string]any) objectclient.KnowledgeScope {
	if repo := flagString(flags, "repo"); repo != "" {
		return objectclient.KnowledgeScope{Repository: repo, Commit: flagString(flags, "commit"), Ref: flagString(flags, "ref")}
	}
	scope := e.context.Scope
	if dataset := flagString(flags, "dataset"); dataset != "" {
		scope.Dataset = dataset
	}
	scope.Repository, scope.Commit, scope.Ref = "", "", ""
	return scope
}

func (e *executor) Execute(ctx context.Context, path string, flags map[string]any, options RequestOptions) (any, error) {
	if _, supported := command(path); !supported {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess CLI does not implement %s", path)
	}
	if err := e.Validate(path, flags); err != nil {
		return nil, err
	}
	if flags["help"] == true {
		help := leafHelpEntries()[path]
		return help.Description + "\n\n" + help.Usage, nil
	}
	if e.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess CLI requires an authenticated HTTP client")
	}
	var output any
	if path == "binding show" {
		err := e.client.ResolveBinding(ctx, BindingRequest{KnowledgeScope: e.scope(flags), Object: flagString(flags, "object"), Aspect: flagString(flags, "aspect"), Member: flagString(flags, "member")}, options, &output)
		return output, err
	}
	request := AccessRequest{KnowledgeScope: e.scope(flags), Object: flagString(flags, "object")}
	if path == "invoke" {
		request.Implementation, request.Operation = flagString(flags, "implementation"), flagString(flags, "operation")
		request.Input = json.RawMessage(flagString(flags, "input"))
	} else {
		request.Aspect, request.Member = flagString(flags, "aspect"), flagString(flags, "member")
	}
	err := e.client.AccessResource(ctx, request, options, &output)
	return output, err
}

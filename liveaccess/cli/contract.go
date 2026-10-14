package cli

import (
	"context"
	accessclient "kc/liveaccess/client"

	objectclient "kc/object/client"
)

type Command struct {
	Path, Handler, Action, Description, Usage string
	Flags, Operands, Groups                   []string
}

type RequestOptions = accessclient.RequestOptions

// Context is a fixed caller basis supplied by the host's existing Dataset pin
// adapter. It contains no authentication proof or arbitrary source endpoint.
type Context struct{ Scope objectclient.KnowledgeScope }

type BindingRequest = accessclient.BindingRequest
type AccessRequest = accessclient.AccessRequest

// Client is the authenticated typed transport supplied by the host. This
// module neither authenticates callers nor constructs a second HTTP stack.
type Client = accessclient.Client

type Executor interface {
	Execute(context.Context, string, map[string]any, RequestOptions) (any, error)
	ValidateFlags(string, map[string]any) error
	Validate(string, map[string]any) error
}

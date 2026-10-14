// Package cli owns declarative-index affordance and maintenance task CLI.
package cli

import (
	"context"
	"kc/declarativeindex/client"
)

type Command struct {
	Path, Handler, Action, Description, Usage string
	Flags, Operands, Groups                   []string
}
type Executor interface {
	Execute(context.Context, string, map[string]any, client.RequestOptions) (any, error)
	ValidateFlags(string, map[string]any) error
	Validate(string, map[string]any) error
}

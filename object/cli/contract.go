// Package cli owns the composable Object command surface and its presentation.
package cli

import (
	"context"
	"kc/object/client"
	"time"
)

type Command struct {
	Path, Handler, Action, Description, Usage string
	Flags, Operands, Groups                   []string
}

// Context contains optional caller defaults, never authentication proof.
type Context struct {
	Dataset     string
	ReceiptWait time.Duration
}

type Executor interface {
	Execute(context.Context, string, map[string]any, client.RequestOptions) (any, error)
	ValidateFlags(string, map[string]any) error
	Validate(string, map[string]any) error
}

// Package cli provides composable Snapshot Store commands over its typed SDK.
package cli

import (
	"context"

	"kc/snapshot/client"
)

// Command is one public leaf and its help. Nil Flags retain the established
// caller-owned flag vocabulary; nonempty Flags restrict that leaf's inputs.
type Command struct {
	Path        string
	Handler     string
	Action      string
	Description string
	Usage       string
	Flags       []string
	// Operands names the flags filled by positional arguments, in order.
	Operands []string
	// Groups lists the module's static task-help groups for this leaf.
	Groups []string
}

// Context is caller-owned command context, never authentication proof.
type Context struct {
	Principal string
}

type Executor interface {
	Execute(context.Context, string, map[string]any, client.RequestOptions) (any, error)
	ValidateFlags(string, map[string]any) error
	Validate(string, map[string]any) error
}

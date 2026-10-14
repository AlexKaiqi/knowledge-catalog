// Package cli owns composable Discovery tasks and their product presentation.
package cli

import (
	"context"
	"encoding/json"
	"kc/discovery/client"
	"kc/snapshot/dataset"
)

type Command struct {
	Path, Handler, Action, Description, Usage string
	Flags, Operands, Groups                   []string
}

// Context carries caller defaults and already fixed scope, never credentials.
type Context struct {
	Dataset            string
	Pin                json.RawMessage
	Definition         *dataset.KnowledgeSet
	InventoryDiscovery bool
}
type Executor interface {
	Execute(context.Context, string, map[string]any, client.RequestOptions) (any, error)
	ValidateFlags(string, map[string]any) error
	Validate(string, map[string]any) error
}

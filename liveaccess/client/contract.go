// Package client supplies LiveAccess tasks over a caller-owned transport.
package client

import (
	"context"
	"encoding/json"
	objectclient "kc/object/client"
)

type Transport = objectclient.Transport
type Config = objectclient.Config
type RequestOptions = objectclient.RequestOptions

type BindingRequest struct {
	objectclient.KnowledgeScope
	Object string `json:"object"`
	Aspect string `json:"aspect"`
	Member string `json:"member,omitempty"`
}

type AccessRequest struct {
	objectclient.KnowledgeScope
	Object         string          `json:"object"`
	Aspect         string          `json:"aspect,omitempty"`
	Member         string          `json:"member,omitempty"`
	Operation      string          `json:"operation,omitempty"`
	Input          json.RawMessage `json:"input,omitempty"`
	Implementation string          `json:"implementation,omitempty"`
}

type Client interface {
	ResolveBinding(context.Context, BindingRequest, RequestOptions, any) error
	AccessResource(context.Context, AccessRequest, RequestOptions, any) error
}

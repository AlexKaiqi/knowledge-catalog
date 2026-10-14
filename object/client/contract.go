// Package client supplies the Object HTTP SDK over a caller-owned transport.
package client

import (
	"context"
	"net/http"
)

type Transport interface {
	Do(context.Context, string, *http.Request) (*http.Response, error)
}

type Config struct {
	BaseURL   string
	Transport Transport
}

type RequestOptions struct{ RequestID string }

type Client interface {
	Read(context.Context, ReadRequest, RequestOptions, any) error
	Resolve(context.Context, ResolveRequest, RequestOptions, any) error
	Log(context.Context, ObjectRequest, RequestOptions, any) error
	Provenance(context.Context, ObjectRequest, RequestOptions, any) error
	Schema(context.Context, SchemaRequest, RequestOptions, any) error
	BrowseSchemas(context.Context, SchemaPageRequest, RequestOptions, any) error
	Commit(context.Context, string, CommitRequest, RequestOptions, any) error
	Put(context.Context, MaintenanceRequest, RequestOptions, any) error
	Remove(context.Context, MaintenanceRequest, RequestOptions, any) error
	Receipt(context.Context, string, RequestOptions, any) error
}

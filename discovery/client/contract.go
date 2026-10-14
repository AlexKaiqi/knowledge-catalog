// Package client supplies the Discovery SDK over caller-owned authentication.
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
	Search(context.Context, SearchRequest, RequestOptions, any) error
	Relations(context.Context, RelationsRequest, RequestOptions, any) error
	Traverse(context.Context, TraverseRequest, RequestOptions, any) error
	DescribeAccessSpec(context.Context, AccessSpecDescribeRequest, RequestOptions, any) error
	PluginHelp(context.Context, string, RequestOptions, any) error
}

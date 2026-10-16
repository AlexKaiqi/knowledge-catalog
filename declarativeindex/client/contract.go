// Package client owns the declarative-index maintenance HTTP SDK.
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
	DescribeProjection(context.Context, ProjectionRequest, RequestOptions, any) error
	SyncProjection(context.Context, ProjectionRequest, RequestOptions, any) error
}
type ProjectionRequest struct {
	Dataset  string `json:"dataset"`
	Revision int    `json:"revision,omitempty"`
}

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

// Client is the index maintenance surface. Readiness, diagnosis and progress
// are three questions about the same state and are deliberately separate
// calls: readiness stays cheap and never carries diagnostic members.
type Client interface {
	IndexReady(context.Context, IndexRequest, RequestOptions, any) error
	IndexPrepare(context.Context, IndexRequest, RequestOptions, any) error
	IndexDescribe(context.Context, IndexRequest, RequestOptions, any) error
	IndexStatus(context.Context, RequestOptions, any) error
}
type IndexRequest struct {
	Dataset  string `json:"dataset"`
	Revision int    `json:"revision,omitempty"`
}

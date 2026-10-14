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
	NotifyProjection(context.Context, ProjectionNotifyRequest, RequestOptions, any) error
}
type ProjectionRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}
type ProjectionNoticeAddress struct {
	Kind       string `json:"kind,omitempty"`
	ObjectID   string `json:"objectId,omitempty"`
	AspectName string `json:"aspectName,omitempty"`
}
type ProjectionNotifyRequest struct {
	Repository     string                   `json:"repository"`
	Ref            string                   `json:"ref,omitempty"`
	Address        *ProjectionNoticeAddress `json:"address,omitempty"`
	SourceRevision string                   `json:"sourceRevision,omitempty"`
}

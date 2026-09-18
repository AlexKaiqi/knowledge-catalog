package client

import (
	"context"
	"encoding/json"
)

type OperationsService struct{ client *Client }

func (c *Client) OperationsService() OperationsService { return OperationsService{client: c} }

type ProjectionSyncRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

// AccessSpecDescribeRequest is the Workspace pin or single Repository used to
// compile logical AccessSpec reports. It is not a Repository projection
// coordinate.
type AccessSpecDescribeRequest struct {
	Catalog    string          `json:"catalog,omitempty"`
	Dataset  string          `json:"dataset"`
	Repository string          `json:"repository,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
}

func (s OperationsService) SyncProjection(ctx context.Context, request ProjectionSyncRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/projections:sync", request, options, output)
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

func (s OperationsService) NotifyProjection(ctx context.Context, request ProjectionNotifyRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/projections:notice", request, options, output)
}

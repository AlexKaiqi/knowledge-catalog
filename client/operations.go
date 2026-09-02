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

// AccessSpecDescribeRequest is the Workspace pin used to compile one logical
// AccessSpec per member. It is not a Repository projection coordinate.
type AccessSpecDescribeRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
}

func (s OperationsService) SyncProjection(ctx context.Context, request ProjectionSyncRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/projections:sync", request, options, output)
}

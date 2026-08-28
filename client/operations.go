package client

import "context"

type OperationsService struct{ client *Client }

func (c *Client) OperationsService() OperationsService { return OperationsService{client: c} }

type ProjectionSyncRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

func (s OperationsService) SyncProjection(ctx context.Context, request ProjectionSyncRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/projections:sync", request, options, output)
}

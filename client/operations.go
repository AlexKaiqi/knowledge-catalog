package client

import (
	"context"
	declarativeindexclient "kc/declarativeindex/client"
	discoveryclient "kc/discovery/client"
)

type OperationsService struct{ client *Client }

func (c *Client) OperationsService() OperationsService { return OperationsService{client: c} }

type ProjectionSyncRequest = declarativeindexclient.ProjectionRequest

// AccessSpecDescribeRequest is the Workspace pin or single Repository used to
// compile logical AccessSpec reports. It is not a Repository projection
// coordinate.
type AccessSpecDescribeRequest = discoveryclient.AccessSpecDescribeRequest

func (s OperationsService) SyncProjection(ctx context.Context, request ProjectionSyncRequest, options RequestOptions, output any) error {
	return s.client.declarativeIndex.SyncProjection(ctx, request, declarativeindexclient.RequestOptions{RequestID: options.RequestID}, output)
}

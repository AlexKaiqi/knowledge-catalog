package client

import (
	"context"
	declarativeindexclient "kc/declarativeindex/client"
	discoveryclient "kc/discovery/client"
)

type OperationsService struct{ client *Client }

func (c *Client) OperationsService() OperationsService { return OperationsService{client: c} }

// IndexRequest names the Dataset revision an index question is about.
type IndexRequest = declarativeindexclient.IndexRequest

// AccessSpecDescribeRequest is the Workspace pin or single Repository used to
// compile logical AccessSpec reports. It is not a Repository projection
// coordinate.
type AccessSpecDescribeRequest = discoveryclient.AccessSpecDescribeRequest

func (s OperationsService) IndexReady(ctx context.Context, request IndexRequest, options RequestOptions, output any) error {
	return s.client.declarativeIndex.IndexReady(ctx, request, declarativeindexclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s OperationsService) IndexPrepare(ctx context.Context, request IndexRequest, options RequestOptions, output any) error {
	return s.client.declarativeIndex.IndexPrepare(ctx, request, declarativeindexclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s OperationsService) IndexDescribe(ctx context.Context, request IndexRequest, options RequestOptions, output any) error {
	return s.client.declarativeIndex.IndexDescribe(ctx, request, declarativeindexclient.RequestOptions{RequestID: options.RequestID}, output)
}

func (s OperationsService) IndexStatus(ctx context.Context, options RequestOptions, output any) error {
	return s.client.declarativeIndex.IndexStatus(ctx, declarativeindexclient.RequestOptions{RequestID: options.RequestID}, output)
}

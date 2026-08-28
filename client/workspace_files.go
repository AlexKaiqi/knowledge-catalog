package client

import (
	"context"
	"encoding/json"
)

type WorkspaceFilesService struct{ client *Client }

func (c *Client) WorkspaceFilesService() WorkspaceFilesService {
	return WorkspaceFilesService{client: c}
}

type WorkspaceFileCoordinate struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
}

type WorkspaceFileMountsRequest struct {
	WorkspaceFileCoordinate
}

type WorkspaceFileDirectoryRequest struct {
	WorkspaceFileCoordinate
	MountPath    string `json:"mountPath"`
	Directory    string `json:"directory,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type WorkspaceFileReadRequest struct {
	WorkspaceFileCoordinate
	MountPath string `json:"mountPath"`
	File      string `json:"file"`
	Offset    int64  `json:"offset,omitempty"`
	Length    int    `json:"length,omitempty"`
}

func (s WorkspaceFilesService) Mounts(ctx context.Context, request WorkspaceFileMountsRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/workspace-files/v1/mounts:list", request, options, output)
}

func (s WorkspaceFilesService) Directory(ctx context.Context, request WorkspaceFileDirectoryRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/workspace-files/v1/tree:list", request, options, output)
}

func (s WorkspaceFilesService) Read(ctx context.Context, request WorkspaceFileReadRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/workspace-files/v1/file:read", request, options, output)
}

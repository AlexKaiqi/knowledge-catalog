package client

import (
	"context"
	"encoding/json"

	"kc/catalog"
	"kc/snapshot"
)

type WorkspaceFilesService struct{ client *Client }

func (c *Client) WorkspaceFilesService() WorkspaceFilesService {
	return WorkspaceFilesService{client: c}
}

type WorkspaceFileCoordinate struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	View      string          `json:"view,omitempty"`
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

type WorkspaceFileMountsResponse struct {
	Pin    catalog.ResolvedWorkspace `json:"pin"`
	Mounts []catalog.VirtualMount    `json:"mounts"`
}

type WorkspaceFileDirectoryResponse struct {
	Pin          catalog.ResolvedWorkspace `json:"pin"`
	Mount        catalog.VirtualMount      `json:"mount"`
	Entries      []snapshot.DirectoryEntry `json:"entries"`
	Continuation string                    `json:"continuation,omitempty"`
	Exhausted    bool                      `json:"exhausted"`
}

type WorkspaceFileReadResponse struct {
	Pin        catalog.ResolvedWorkspace `json:"pin"`
	Mount      catalog.VirtualMount      `json:"mount"`
	File       string                    `json:"file"`
	Offset     int64                     `json:"offset"`
	TotalBytes int64                     `json:"totalBytes"`
	EOF        bool                      `json:"eof"`
	Content    []byte                    `json:"content"`
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

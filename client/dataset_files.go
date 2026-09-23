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
	Catalog string          `json:"catalog,omitempty"`
	Dataset string          `json:"dataset"`
	Pin     json.RawMessage `json:"pin,omitempty"`
	View    string          `json:"view,omitempty"`
}

type WorkspaceFileMountsRequest struct {
	WorkspaceFileCoordinate
}

type WorkspaceFileDirectoryRequest struct {
	WorkspaceFileCoordinate
	// MountPath+Directory address one mount-relative subtree. Path addresses
	// the composed delivered tree directly (per-file entries included); the
	// two modes are mutually exclusive.
	MountPath    string `json:"mountPath"`
	Directory    string `json:"directory,omitempty"`
	Path         string `json:"path,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type WorkspaceFileReadRequest struct {
	WorkspaceFileCoordinate
	MountPath string `json:"mountPath"`
	File      string `json:"file"`
	// Path addresses one delivered file in the composed tree (see
	// WorkspaceFileDirectoryRequest).
	Path   string `json:"path,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
}

type WorkspaceFileMountsResponse struct {
	Pin    catalog.ResolvedKnowledgeSet `json:"pin"`
	Mounts []catalog.VirtualMount       `json:"mounts"`
}

type WorkspaceFileDirectoryResponse struct {
	Pin          catalog.ResolvedKnowledgeSet `json:"pin"`
	Mount        catalog.VirtualMount         `json:"mount"`
	Entries      []snapshot.DirectoryEntry    `json:"entries"`
	Continuation string                       `json:"continuation,omitempty"`
	Exhausted    bool                         `json:"exhausted"`
}

type WorkspaceFileReadResponse struct {
	Pin        catalog.ResolvedKnowledgeSet `json:"pin"`
	Mount      catalog.VirtualMount         `json:"mount"`
	File       string                       `json:"file"`
	// Path echoes the delivered path when the request addressed the composed
	// tree; Item carries the per-file entry that served the bytes.
	Path       string               `json:"path,omitempty"`
	Item       *catalog.DatasetItem `json:"item,omitempty"`
	Offset     int64                `json:"offset"`
	TotalBytes int64                `json:"totalBytes"`
	EOF        bool                 `json:"eof"`
	Content    []byte               `json:"content"`
}

func (s WorkspaceFilesService) Mounts(ctx context.Context, request WorkspaceFileMountsRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/dataset-files/v1/mounts:list", request, options, output)
}

func (s WorkspaceFilesService) Directory(ctx context.Context, request WorkspaceFileDirectoryRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/dataset-files/v1/tree:list", request, options, output)
}

func (s WorkspaceFilesService) Read(ctx context.Context, request WorkspaceFileReadRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/dataset-files/v1/file:read", request, options, output)
}

package client

import (
	"context"
	"encoding/json"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type WorkspaceFileCoordinate struct {
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
	// Repository+Commit select raw fixed file access instead of a Dataset.
	// This mode uses Path and rejects Dataset, Pin, View, MountPath and File.
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	MountPath  string `json:"mountPath"`
	File       string `json:"file"`
	// Path addresses one delivered file in the composed tree (see
	// WorkspaceFileDirectoryRequest).
	Path   string `json:"path,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
}

type WorkspaceFileMountsResponse struct {
	Pin    dataset.ResolvedDataset `json:"pin"`
	Mounts []dataset.VirtualMount  `json:"mounts"`
}

type WorkspaceFileDirectoryResponse struct {
	Pin          dataset.ResolvedDataset   `json:"pin"`
	Mount        dataset.VirtualMount      `json:"mount"`
	Entries      []snapshot.DirectoryEntry `json:"entries"`
	Continuation string                    `json:"continuation,omitempty"`
	Exhausted    bool                      `json:"exhausted"`
}

type WorkspaceFileReadResponse struct {
	Pin   dataset.ResolvedDataset `json:"pin"`
	Mount dataset.VirtualMount    `json:"mount"`
	File  string                  `json:"file"`
	// Path echoes the delivered path when the request addressed the composed
	// tree; Item carries the per-file entry that served the bytes.
	Path       string               `json:"path,omitempty"`
	Item       *dataset.DatasetItem `json:"item,omitempty"`
	Offset     int64                `json:"offset"`
	TotalBytes int64                `json:"totalBytes"`
	EOF        bool                 `json:"eof"`
	Content    []byte               `json:"content"`
}

// RepositoryFileReadResponse is returned for Repository+Commit reads. A raw
// source file has no Dataset pin or delivery-layout coordinates.
type RepositoryFileReadResponse struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Path       string              `json:"path"`
	Offset     int64               `json:"offset"`
	TotalBytes int64               `json:"totalBytes"`
	EOF        bool                `json:"eof"`
	Content    []byte              `json:"content"`
}

func (s filesService) Mounts(ctx context.Context, request WorkspaceFileMountsRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/files/mounts:list", request, options, output)
}

func (s filesService) Directory(ctx context.Context, request WorkspaceFileDirectoryRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/files/tree:list", request, options, output)
}

func (s filesService) Read(ctx context.Context, request WorkspaceFileReadRequest, options RequestOptions, output any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/files/file:read", request, options, output)
}

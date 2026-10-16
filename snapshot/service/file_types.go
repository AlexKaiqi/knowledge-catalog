package service

import (
	"encoding/json"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type FileCoordinate struct {
	Dataset string          `json:"dataset"`
	Pin     json.RawMessage `json:"pin,omitempty"`
	View    string          `json:"view,omitempty"`
}

type FileMountsRequest struct {
	FileCoordinate
}

type FileDirectoryRequest struct {
	FileCoordinate
	// MountPath+Directory address one mount-relative subtree (kcfs shape).
	// Path addresses the composed delivered tree directly, which is how
	// per-file entries (U10) outside any mount stay reachable. The two
	// addressing modes are mutually exclusive.
	MountPath    string `json:"mountPath"`
	Directory    string `json:"directory,omitempty"`
	Path         string `json:"path,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type FileReadRequest struct {
	FileCoordinate
	// Repository+Commit address a fixed raw file, exclusively of Dataset,
	// pin, view and mount-relative coordinates. Path is then a repository path.
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	MountPath  string `json:"mountPath"`
	File       string `json:"file"`
	// Path addresses one delivered file in the composed tree (see
	// FileDirectoryRequest). MountPath+File remain the mount shape.
	Path   string `json:"path,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
}

type FileMountsResponse struct {
	Pin    dataset.ResolvedDataset `json:"pin"`
	Mounts []dataset.VirtualMount  `json:"mounts"`
}

type FileDirectoryResponse struct {
	Pin          dataset.ResolvedDataset   `json:"pin"`
	Mount        dataset.VirtualMount      `json:"mount"`
	Entries      []snapshot.DirectoryEntry `json:"entries"`
	Continuation string                    `json:"continuation,omitempty"`
	Exhausted    bool                      `json:"exhausted"`
}

type FileReadResponse struct {
	Pin   dataset.ResolvedDataset `json:"pin"`
	Mount dataset.VirtualMount    `json:"mount"`
	File  string                  `json:"file"`
	// Path echoes the delivered path: mount-relative reads report
	// mountPath/file joined; delivered reads echo the requested path.
	Path       string               `json:"path,omitempty"`
	Item       *dataset.DatasetItem `json:"item,omitempty"`
	Offset     int64                `json:"offset"`
	TotalBytes int64                `json:"totalBytes"`
	EOF        bool                 `json:"eof"`
	Content    []byte               `json:"content"`
}

// RepositoryFileReadResponse reports immutable source coordinates without a
// Dataset publication. Authorization is checked again for each request.
type RepositoryFileReadResponse struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Path       string              `json:"path"`
	Offset     int64               `json:"offset"`
	TotalBytes int64               `json:"totalBytes"`
	EOF        bool                `json:"eof"`
	Content    []byte              `json:"content"`
}

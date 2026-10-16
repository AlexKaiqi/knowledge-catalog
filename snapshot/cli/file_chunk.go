package cli

import (
	"context"

	"kc/kernel"
	kcclient "kc/snapshot/client"
	"kc/snapshot/dataset"
)

type fileChunkSource struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	File       string
	Mount      dataset.VirtualMount
}

type fileChunk struct {
	Source     fileChunkSource
	Offset     int64
	TotalBytes int64
	EOF        bool
	Content    []byte
}

func readFileChunk(ctx context.Context, service kcclient.FilesService, request kcclient.WorkspaceFileReadRequest, pin dataset.ResolvedDataset, options kcclient.RequestOptions) (fileChunk, error) {
	var chunk fileChunk
	var err error
	if request.Repository != "" {
		chunk, err = readRepositoryChunk(ctx, service, request, options)
	} else {
		chunk, err = readDeliveredChunk(ctx, service, request, pin, options)
	}
	if err != nil {
		return chunk, err
	}
	if chunk.TotalBytes < 0 || chunk.Offset != min(request.Offset, chunk.TotalBytes) || len(chunk.Content) > request.Length ||
		int64(len(chunk.Content)) > chunk.TotalBytes-chunk.Offset || chunk.EOF != (int64(len(chunk.Content)) == chunk.TotalBytes-chunk.Offset) || (!chunk.EOF && len(chunk.Content) == 0) {
		return chunk, kernel.Fail(kernel.ErrPreconditionFailed, "file response changed its byte range")
	}
	return chunk, nil
}

func readRepositoryChunk(ctx context.Context, service kcclient.FilesService, request kcclient.WorkspaceFileReadRequest, options kcclient.RequestOptions) (fileChunk, error) {
	var read kcclient.RepositoryFileReadResponse
	if err := service.Read(ctx, request, options, &read); err != nil {
		return fileChunk{}, err
	}
	if string(read.Repository) != request.Repository || string(read.Commit) != request.Commit || read.Path != request.Path {
		return fileChunk{}, kernel.Fail(kernel.ErrPreconditionFailed, "file response changed its repository, commit or path")
	}
	return fileChunk{Source: fileChunkSource{Repository: read.Repository, Commit: read.Commit, File: read.Path},
		Offset: read.Offset, TotalBytes: read.TotalBytes, EOF: read.EOF, Content: read.Content}, nil
}

func readDeliveredChunk(ctx context.Context, service kcclient.FilesService, request kcclient.WorkspaceFileReadRequest, pin dataset.ResolvedDataset, options kcclient.RequestOptions) (fileChunk, error) {
	var read kcclient.WorkspaceFileReadResponse
	if err := service.Read(ctx, request, options, &read); err != nil {
		return fileChunk{}, err
	}
	if !sameFilePublication(pin, read.Pin) || read.Path != request.Path || read.Mount.Repository == "" || pin.Repositories[read.Mount.Repository] != read.Mount.Commit || read.Mount.Commit == "" {
		return fileChunk{}, kernel.Fail(kernel.ErrPreconditionFailed, "file response changed its publication or path")
	}
	return fileChunk{Source: fileChunkSource{Repository: read.Mount.Repository, Commit: read.Mount.Commit, File: read.File, Mount: read.Mount},
		Offset: read.Offset, TotalBytes: read.TotalBytes, EOF: read.EOF, Content: read.Content}, nil
}

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"kc/kernel"
	kcclient "kc/snapshot/client"
	"kc/snapshot/dataset"
)

func downloadFile(ctx context.Context, service kcclient.FilesService, request kcclient.WorkspaceFileReadRequest, pin dataset.ResolvedDataset, target string, options kcclient.RequestOptions, output map[string]any) (any, error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".kc-file-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = temporary.Close(); _ = os.Remove(temporary.Name()) }()
	request.Offset, request.Length = 0, fileReadChunkSize
	total := int64(-1)
	var source fileChunkSource
	for {
		read, err := readFileChunk(ctx, service, request, pin, options)
		if err != nil {
			return nil, err
		}
		if total >= 0 && (read.TotalBytes != total || read.Source != source) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "fixed file source or size changed during download")
		}
		total, source = read.TotalBytes, read.Source
		if _, err := temporary.Write(read.Content); err != nil {
			return nil, err
		}
		request.Offset += int64(len(read.Content))
		if read.EOF {
			break
		}
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	// Publish complete bytes atomically without replacing a concurrently created target.
	if err := os.Link(temporary.Name(), target); err != nil {
		if os.IsExist(err) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "file target already exists: %s", target)
		}
		return nil, fmt.Errorf("publish complete file without replacement: %w", err)
	}
	output["out"], output["bytes"] = target, request.Offset
	return output, nil
}

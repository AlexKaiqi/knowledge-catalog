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

const fileReadChunkSize = 512 << 10

func runRemoteFileRead(ctx context.Context, client kcclient.Client, id string, revision *int, delivered string, flags map[string]any, options kcclient.RequestOptions) (any, error) {
	offset, length, target, err := fileReadControls(flags)
	if err != nil {
		return nil, err
	}
	coordinate, pin, err := resolveFilesCoordinate(ctx, client, id, revision, options)
	if err != nil {
		return nil, err
	}
	request := kcclient.WorkspaceFileReadRequest{WorkspaceFileCoordinate: coordinate, Path: delivered, Offset: offset, Length: int(length)}
	if target != "" {
		return downloadDeliveredFile(ctx, client.WorkspaceFilesService(), request, pin, target, options)
	}
	read, err := readDeliveredChunk(ctx, client.WorkspaceFilesService(), request, pin, options)
	if err != nil {
		return nil, err
	}
	output := filePublicationOutput(pin, delivered)
	if read.Content == nil {
		read.Content = []byte{}
	}
	output["offset"], output["totalBytes"], output["eof"], output["content"] = read.Offset, read.TotalBytes, read.EOF, read.Content
	output["repository"], output["commit"] = read.Mount.Repository, read.Mount.Commit
	return output, nil
}

func fileReadControls(flags map[string]any) (int64, int64, string, error) {
	offset, err := optionalFileNumber(flags, "offset", 0)
	if err != nil {
		return 0, 0, "", err
	}
	length, err := optionalFileNumber(flags, "length", fileReadChunkSize)
	if err != nil {
		return 0, 0, "", err
	}
	if length > 4<<20 {
		return 0, 0, "", kernel.Fail(kernel.ErrUsageInvalid, "--length must be no greater than 4 MiB")
	}
	if length == 0 {
		length = fileReadChunkSize
	}
	target := flagString(flags, "out")
	if _, present := flags["out"]; present {
		_, hasOffset := flags["offset"]
		_, hasLength := flags["length"]
		if target == "" || hasOffset || hasLength {
			return 0, 0, "", kernel.Fail(kernel.ErrUsageInvalid, "--out requires a file target and cannot be combined with --offset or --length")
		}
		if err := fileTargetAbsent(target); err != nil {
			return 0, 0, "", err
		}
	}
	return offset, length, target, nil
}

func readDeliveredChunk(ctx context.Context, service kcclient.FilesService, request kcclient.WorkspaceFileReadRequest, pin dataset.ResolvedDataset, options kcclient.RequestOptions) (kcclient.WorkspaceFileReadResponse, error) {
	var read kcclient.WorkspaceFileReadResponse
	if err := service.Read(ctx, request, options, &read); err != nil {
		return read, err
	}
	expectedOffset := min(request.Offset, read.TotalBytes)
	if !sameFilePublication(pin, read.Pin) || read.Path != request.Path || read.Mount.Repository == "" || pin.Repositories[read.Mount.Repository] != read.Mount.Commit || read.Mount.Commit == "" || read.TotalBytes < 0 || read.Offset != expectedOffset ||
		len(read.Content) > request.Length || int64(len(read.Content)) > read.TotalBytes-read.Offset || read.EOF != (int64(len(read.Content)) == read.TotalBytes-read.Offset) || (!read.EOF && len(read.Content) == 0) {
		return read, kernel.Fail(kernel.ErrPreconditionFailed, "file response changed its publication, path or byte range")
	}
	return read, nil
}

func fileTargetAbsent(target string) error {
	_, err := os.Lstat(target)
	if err == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "file target already exists: %s", target)
	}
	if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func downloadDeliveredFile(ctx context.Context, service kcclient.FilesService, request kcclient.WorkspaceFileReadRequest, pin dataset.ResolvedDataset, target string, options kcclient.RequestOptions) (any, error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".kc-file-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = temporary.Close(); _ = os.Remove(temporary.Name()) }()
	request.Offset, request.Length = 0, fileReadChunkSize
	total := int64(-1)
	var fixedMount dataset.VirtualMount
	var fixedFile string
	for {
		read, err := readDeliveredChunk(ctx, service, request, pin, options)
		if err != nil {
			return nil, err
		}
		if total >= 0 && (read.TotalBytes != total || read.Mount != fixedMount || read.File != fixedFile) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "fixed file source or size changed during download")
		}
		total, fixedMount, fixedFile = read.TotalBytes, read.Mount, read.File
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
	// Same-directory hard linking publishes the completed bytes atomically and
	// refuses an existing target, including one created after the initial check.
	if err := os.Link(temporary.Name(), target); err != nil {
		if os.IsExist(err) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "file target already exists: %s", target)
		}
		return nil, fmt.Errorf("publish complete file without replacement: %w", err)
	}
	output := filePublicationOutput(pin, request.Path)
	output["out"], output["bytes"] = target, request.Offset
	return output, nil
}

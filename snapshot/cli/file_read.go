package cli

import (
	"context"
	"os"

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
	request := kcclient.WorkspaceFileReadRequest{Repository: flagString(flags, "repo"), Commit: flagString(flags, "commit"), Path: delivered, Offset: offset, Length: int(length)}
	var pin dataset.ResolvedDataset
	if request.Repository == "" {
		request.WorkspaceFileCoordinate, pin, err = resolveFilesCoordinate(ctx, client, id, revision, options)
		if err != nil {
			return nil, err
		}
	}
	output := fileReadOutput(request, pin)
	if target != "" {
		return downloadFile(ctx, client.WorkspaceFilesService(), request, pin, target, options, output)
	}
	read, err := readFileChunk(ctx, client.WorkspaceFilesService(), request, pin, options)
	if err != nil {
		return nil, err
	}
	if read.Content == nil {
		read.Content = []byte{}
	}
	output["offset"], output["totalBytes"], output["eof"], output["content"] = read.Offset, read.TotalBytes, read.EOF, read.Content
	output["repository"], output["commit"] = read.Source.Repository, read.Source.Commit
	return output, nil
}

func fileReadOutput(request kcclient.WorkspaceFileReadRequest, pin dataset.ResolvedDataset) map[string]any {
	if request.Repository != "" {
		return map[string]any{"repository": request.Repository, "commit": request.Commit, "path": request.Path}
	}
	return filePublicationOutput(pin, request.Path)
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

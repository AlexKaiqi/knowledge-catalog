package service

import (
	"context"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
)

func (resources Resources) readRepositoryFile(ctx context.Context, caller identity.HTTPIdentity, request FileReadRequest) (RepositoryFileReadResponse, error) {
	var response RepositoryFileReadResponse
	if strings.TrimSpace(request.Repository) == "" || strings.TrimSpace(request.Commit) == "" || !ValidRelativePath(request.Path) {
		return response, kernel.Fail(kernel.ErrUsageInvalid, "repository file reads require repository, fixed commit and relative path")
	}
	if request.Dataset != "" || len(request.Pin) != 0 || request.View != "" || request.MountPath != "" || request.File != "" {
		return response, kernel.Fail(kernel.ErrUsageInvalid, "repository file coordinates cannot be combined with Dataset or mount coordinates")
	}
	byteRange, err := requestedFileRange(request.Offset, request.Length)
	if err != nil {
		return response, err
	}
	query := access.Query{Action: access.RepositoryFileRead, Repo: request.Repository, Ref: request.Commit}
	if err := resources.authorize(ctx, Authorization{Caller: caller, NativeRepository: true, Query: query}); err != nil {
		return response, err
	}
	repository, commit := kernel.RepositoryID(request.Repository), kernel.CommitID(request.Commit)
	content, readErr := readRepositoryBytes(resources.Sources, repository, commit, request.Path)
	if resources.RecordRead != nil {
		evidence := FileReadEvidence{Repository: repository, Commit: commit, Path: request.Path, Bytes: len(content), Err: readErr}
		if err := resources.RecordRead(ctx, caller, evidence); err != nil && readErr == nil {
			return response, err
		}
	}
	if readErr != nil {
		return response, readErr
	}
	offset, chunk, eof := byteRange.slice(content)
	return RepositoryFileReadResponse{Repository: repository, Commit: commit, Path: request.Path,
		Offset: offset, TotalBytes: int64(len(content)), EOF: eof, Content: chunk}, nil
}

func readRepositoryBytes(sources Sources, repository kernel.RepositoryID, commit kernel.CommitID, path string) ([]byte, error) {
	if sources == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "file source authority is required")
	}
	store, ok := sources.Get(repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository %s is not attached", repository)
	}
	if !store.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	reader, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s does not support fixed file reads", repository)
	}
	return reader.ReadFile(path, commit)
}

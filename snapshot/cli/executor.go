package cli

import (
	"context"
	"strings"

	"kc/kernel"
	"kc/snapshot/client"
)

type executor struct {
	client  client.Client
	context Context
}

func New(remote client.Client, context Context) Executor {
	return &executor{client: remote, context: context}
}

func (e *executor) Execute(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	if !hasCommand(path) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Snapshot CLI does not implement %s", path)
	}
	if err := e.Validate(path, flags); err != nil {
		return nil, err
	}
	if flags["help"] == true {
		for _, command := range commandEntries {
			if command.Path == path {
				return command.Description + "\n\n" + command.Usage, nil
			}
		}
	}
	if path == "dataset overlay" {
		return clientDatasetOverlay(flags)
	}
	if e.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Snapshot CLI requires an authenticated HTTP client")
	}
	switch {
	case strings.HasPrefix(path, "dataset "):
		return e.dataset(ctx, path, flags, options)
	case strings.HasPrefix(path, "files "):
		return runRemoteFiles(ctx, e.client, path, flags, options)
	case strings.HasPrefix(path, "grant "):
		return e.grants(ctx, path, flags, options)
	default:
		return e.registry(ctx, path, flags, options)
	}
}

func (e *executor) dataset(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	service := e.client.SnapshotService()
	var output any
	var err error
	switch path {
	case "dataset define":
		definition, inputErr := remoteDatasetInput(flags)
		if inputErr != nil {
			return nil, inputErr
		}
		revision, _ := remoteIntFlag(flags, "revision")
		err = service.DefineDataset(ctx, client.DatasetRequest{Dataset: flagString(flags, "dataset"), Revision: revision, Sources: definition.Sources, Metadata: definition.Metadata}, options, &output)
	case "dataset show":
		return runRemoteDatasetShow(ctx, service, flags, options)
	case "dataset retire":
		err = service.RetireDataset(ctx, flagString(flags, "dataset"), options, &output)
	}
	return output, err
}

func (e *executor) registry(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	service := e.client.SnapshotService()
	var output any
	var err error
	switch path {
	case "show":
		err = service.Show(ctx, options, &output)
	case "registry audit":
		limit, inputErr := remoteLimit(flags)
		if inputErr != nil {
			return nil, inputErr
		}
		err = service.Audit(ctx, limit, flagString(flags, "continuation"), options, &output)
	case "attach":
		err = service.AttachRepository(ctx, client.RepositoryAttachRequest{Repository: flagString(flags, "repo")}, options, &output)
	case "detach":
		err = service.DetachRepository(ctx, flagString(flags, "repo"), options, &output)
	}
	return output, err
}

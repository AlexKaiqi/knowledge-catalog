package cli

import (
	"context"

	kcclient "kc/snapshot/client"
)

func runRemoteDatasetShow(ctx context.Context, service kcclient.Service, flags map[string]any, options kcclient.RequestOptions) (any, error) {
	id, query, err := datasetShowInput(flags)
	if err != nil {
		return nil, err
	}
	var output any
	err = service.ShowDataset(ctx, id, query, options, &output)
	return output, err
}

func datasetShowInput(flags map[string]any) (string, kcclient.DatasetShowOptions, error) {
	id, err := requireRemoteFlag(flags, "dataset")
	if err != nil {
		return "", kcclient.DatasetShowOptions{}, err
	}
	revision, err := snapshotRevision(flags)
	if err != nil {
		return "", kcclient.DatasetShowOptions{}, err
	}
	limit, err := pageLimit(flags, defaultHistoryLimit, maxHistoryPageSize)
	if err != nil {
		return "", kcclient.DatasetShowOptions{}, err
	}
	return id, kcclient.DatasetShowOptions{Revision: revision, Limit: limit, Continuation: flagString(flags, "continuation")}, nil
}

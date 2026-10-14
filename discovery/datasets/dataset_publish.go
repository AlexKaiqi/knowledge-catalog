package datasets

import (
	"context"
	"kc/kernel"
	"kc/snapshot/dataset"
	"maps"
)

type DatasetPublication struct {
	Dataset  string
	Revision int
	Sources  []dataset.DatasetSource
	Metadata map[string]string
}

type DatasetRegistry interface {
	PrepareDataset(string, int, []dataset.DatasetSource) (dataset.Dataset, error)
	PublishDataset(dataset.Dataset) (dataset.Dataset, error)
}

// DatasetPublisher orders authorization, frozen candidate, capability readiness,
// and the single authoritative serving switch. Readiness cannot mutate Dataset service.
type DatasetPublisher struct {
	Registry  DatasetRegistry
	Authorize func(context.Context, DatasetPublication) error
	Prepare   func(context.Context, dataset.Dataset) error
}

func (e DatasetPublisher) Execute(ctx context.Context, request DatasetPublication) (dataset.Dataset, error) {
	if e.Registry == nil || e.Authorize == nil || e.Prepare == nil {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication services are incomplete")
	}
	if err := e.Authorize(ctx, request); err != nil {
		return dataset.Dataset{}, err
	}
	def, err := e.Registry.PrepareDataset(request.Dataset, request.Revision, request.Sources)
	if err != nil {
		return dataset.Dataset{}, err
	}
	def.Metadata = maps.Clone(request.Metadata)
	if err := e.Prepare(ctx, def); err != nil {
		return dataset.Dataset{}, err
	}
	if err := ctx.Err(); err != nil {
		return dataset.Dataset{}, err
	}
	return e.Registry.PublishDataset(def)
}

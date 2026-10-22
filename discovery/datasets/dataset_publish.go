package datasets

import (
	"context"
	"errors"
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
	Prepare   func(context.Context, dataset.Dataset) (dataset.Dataset, error)
	// PrepareResources returns the frozen candidate and its exact resource ticket.
	// It replaces Prepare for managed publication, and requires coordination.
	PrepareResources func(context.Context, dataset.Dataset) (dataset.Dataset, PublicationProtection, error)
}

func (e DatasetPublisher) Execute(ctx context.Context, request DatasetPublication) (dataset.Dataset, error) {
	if e.Registry == nil || e.Authorize == nil || (e.Prepare == nil && e.PrepareResources == nil) {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication services are incomplete")
	}
	if err := e.Authorize(ctx, request); err != nil {
		return dataset.Dataset{}, err
	}
	if e.Prepare != nil && e.PrepareResources != nil {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrUsageInvalid, "dataset publication requires one preparation path")
	}
	if e.PrepareResources != nil {
		_, ok := e.Registry.(dataset.PublicationCoordinator)
		if !ok {
			return dataset.Dataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "protected Dataset publication requires authority coordination")
		}
	}
	def, err := e.Registry.PrepareDataset(request.Dataset, request.Revision, request.Sources)
	if err != nil {
		return dataset.Dataset{}, err
	}
	def.Metadata = maps.Clone(request.Metadata)
	identity := publicationIdentity(def)
	var prepared dataset.Dataset
	var ticket PublicationProtection
	if e.PrepareResources != nil {
		prepared, ticket, err = e.PrepareResources(ctx, def)
	} else {
		prepared, err = e.Prepare(ctx, def)
	}
	if ticket != nil && err != nil {
		err = errors.Join(err, ticket.Cancel(ctx), ticket.Close())
	}
	if err != nil {
		return dataset.Dataset{}, err
	}
	if publicationIdentity(def) != identity || publicationIdentity(prepared) != identity {
		err := kernel.Fail(kernel.ErrPreconditionFailed, "capability preparation changed the frozen publication candidate")
		if ticket != nil {
			err = errors.Join(err, ticket.Cancel(ctx), ticket.Close())
		}
		return dataset.Dataset{}, err
	}
	def.Dependencies = prepared.Dependencies
	if err := ctx.Err(); err != nil {
		if ticket != nil {
			err = errors.Join(err, ticket.Cancel(ctx), ticket.Close())
		}
		return dataset.Dataset{}, err
	}
	return e.publishPrepared(ctx, def, ticket)
}

// Dependencies are the only preparation output allowed to change the frozen
// candidate. Capture this value before callbacks: slices, pointers and maps
// otherwise let an in-place mutation change both sides of an equality check.
func publicationIdentity(def dataset.Dataset) kernel.Digest {
	def.Dependencies = nil
	return kernel.CanonicalDigest(def)
}

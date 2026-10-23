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

// DatasetPublisher orders authorization, frozen candidate, frozen reference
// dependencies, range validation and the single authoritative acceptance.
// It never builds a retrieval corpus: acceptance is a metadata decision, and
// the corpus for the accepted release is built afterwards as that release's
// own background task (doc/design/dataset.md#发布).
type DatasetPublisher struct {
	Registry  DatasetRegistry
	Authorize func(context.Context, DatasetPublication) error
	// Prepare validates the frozen candidate: it freezes reference dependencies
	// and proves the range takes no Aspect of one object from two sources. It
	// may not change the candidate's identity or sources, and it must not build
	// derived capabilities, because the caller's acceptance cannot wait for one.
	Prepare func(context.Context, dataset.Dataset) (dataset.Dataset, error)
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
	identity := publicationIdentity(def)
	prepared, err := e.Prepare(ctx, def)
	if err != nil {
		return dataset.Dataset{}, err
	}
	if publicationIdentity(def) != identity || publicationIdentity(prepared) != identity {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrPreconditionFailed, "capability preparation changed the frozen publication candidate")
	}
	def.Dependencies = prepared.Dependencies
	if err := ctx.Err(); err != nil {
		return dataset.Dataset{}, err
	}
	return e.Registry.PublishDataset(def)
}

// Dependencies are the only preparation output allowed to change the frozen
// candidate. Capture this value before callbacks: slices, pointers and maps
// otherwise let an in-place mutation change both sides of an equality check.
func publicationIdentity(def dataset.Dataset) kernel.Digest {
	def.Dependencies = nil
	return kernel.CanonicalDigest(def)
}

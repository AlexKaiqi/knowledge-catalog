package home

import (
	"context"

	"kc/internal/datasetbridge"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// prepareDataset builds the corpus of one accepted release. It is the
// background task of acceptance and the explicit retry path, never part of the
// publication request (doc/design/dataset.md#发布).
func (ws *Home) prepareDataset(lookup reader.MemberLookup) func(context.Context, dataset.ResolvedDataset) error {
	return func(ctx context.Context, candidate dataset.ResolvedDataset) error {
		return ws.Plugins.PrepareDatasetScope(ctx, lookup, DatasetPin(candidate))
	}
}

// DatasetPin is the one conversion from a Dataset service resolution to the
// object reader's consumer basis. The mapping itself lives in
// internal/datasetbridge so Home, fixtures and plugin tests cannot drift.
func DatasetPin(resolved dataset.ResolvedDataset) reader.DatasetPin {
	return datasetbridge.Pin(resolved)
}

// publicationDependencies freezes, for a candidate, the versions of the
// Repositories that hold references its selected units make but it does not
// select (doc/design/dataset.md#发布).
func publicationDependencies(lookup func(kernel.RepositoryID) (objectmodel.Repository, error), inventory reader.RepositoryInventory) func(dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) {
	return func(candidate dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) {
		candidate.Dependencies = nil
		return reader.Open(reader.MemberLookup(lookup), DatasetPin(candidate)).FreezeDependencies(inventory)
	}
}

// publicationConflicts is the Dataset publication object-identity check: it
// reads a frozen candidate as a pin and rejects one Aspect of one object taken
// from two source Repositories. bound adds Schema-bound Aspects.
func publicationConflicts(lookup func(kernel.RepositoryID) (objectmodel.Repository, error)) func(dataset.ResolvedDataset) error {
	return func(candidate dataset.ResolvedDataset) error {
		return reader.Open(reader.MemberLookup(lookup), DatasetPin(candidate)).CheckHoldingConflicts()
	}
}

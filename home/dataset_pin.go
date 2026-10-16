package home

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

func (ws *Home) prepareDataset(lookup reader.MemberLookup) func(context.Context, dataset.ResolvedDataset) error {
	return func(ctx context.Context, candidate dataset.ResolvedDataset) error {
		return ws.Plugins.PrepareDatasetScope(ctx, lookup, DatasetPin(candidate))
	}
}

// DatasetPin is the one conversion from a Dataset service resolution to the
// object reader's consumer basis.
func DatasetPin(resolved dataset.ResolvedDataset) reader.DatasetPin {
	items := make([]reader.DatasetItem, 0, len(resolved.Items))
	for _, item := range resolved.Items {
		items = append(items, reader.DatasetItem{
			Target: item.Target, Repository: item.Repository, Commit: item.Commit,
			Kind: item.Kind, Prefix: item.Prefix, File: item.File,
		})
	}
	return reader.DatasetPin{SetID: resolved.SetID, Revision: resolved.Revision, Repositories: resolved.Repositories, Items: items, Dependencies: resolved.Dependencies}
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

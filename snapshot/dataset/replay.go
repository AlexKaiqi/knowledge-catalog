package dataset

import (
	"maps"
	"slices"

	"kc/kernel"
)

// ReplayPublished treats a pin only as a selector for an accepted release.
// Neither its repository commits nor its file list can grant new scope.
func (c *service) ReplayPublished(pin ResolvedDataset) (ResolvedDataset, error) {
	def, err := c.DatasetVersion(pin.SetID, pin.Revision)
	if err != nil {
		return ResolvedDataset{}, err
	}
	resolved, err := ReplayDefinition(def, pin)
	if err != nil {
		return ResolvedDataset{}, err
	}
	// Replay reconstructs authority coordinates before a consumer's per-source
	// authorization. It must not read selected file bodies as an availability
	// probe; explicit CheckResolved owns that check after its caller authorizes it.
	for repository, commit := range resolved.Repositories {
		if len(c.checkResolvedRepository(repository, commit)) != 0 {
			return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "published pin is no longer available")
		}
	}
	return resolved, nil
}

// ReplayDefinition validates an explicitly authorized recipe and reconstructs
// its file list. For named Dataset grants use ReplayPublished, not this helper.
func ReplayDefinition(def Dataset, pin ResolvedDataset) (ResolvedDataset, error) {
	if def.Retired {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "dataset is retired")
	}
	if pin.SetID != "" && pin.SetID != def.SetID {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return ResolvedDataset{}, err
	}
	if err := validateSourceCoordinates(def.Sources); err != nil {
		return ResolvedDataset{}, err
	}
	want := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		commit, exists := pin.Repositories[src.Repository]
		if !exists {
			return ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "pin membership does not match dataset")
		}
		if commit == "" || (src.Commit != "" && src.Commit != commit) {
			return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "pin commit is not the published source commit")
		}
		want[src.Repository] = commit
	}
	if !maps.Equal(want, pin.Repositories) || len(want) == 0 {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "pin membership does not match dataset")
	}
	items, err := datasetItemsFromSources(def.Sources, want)
	if err != nil {
		return ResolvedDataset{}, err
	}
	id := pinIdentity(def, want)
	if (pin.PinID != "" && pin.PinID != id) || (len(pin.Items) > 0 && !slices.Equal(pin.Items, items)) {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "pin scope does not match dataset")
	}
	if pin.Ref != "" && pin.Ref != DatasetVersionRef(def.Revision) {
		return ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "pin version does not match dataset")
	}
	return ResolvedDataset{SetID: def.SetID, Revision: def.Revision, Ref: DatasetVersionRef(def.Revision), Repositories: want, Items: items, PinID: id, Dependencies: maps.Clone(def.Dependencies)}, nil
}

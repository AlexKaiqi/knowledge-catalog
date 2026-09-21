package catalog

import (
	"maps"
	"slices"

	"kc/kernel"
)

// ReplayPublished treats a pin only as a selector for an accepted release.
// Neither its repository commits nor its file list can grant new scope.
func (c *Catalog) ReplayPublished(pin ResolvedKnowledgeSet) (ResolvedKnowledgeSet, error) {
	def, err := c.DatasetVersion(pin.SetID, pin.Revision)
	if err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	resolved, err := ReplayDefinition(def, pin)
	if err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	if check := c.CheckResolved(resolved); check.Outcome != "PASSED" {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "published pin is no longer available")
	}
	return resolved, nil
}

// ReplayDefinition validates an explicitly authorized recipe and reconstructs
// its file list. For named Dataset grants use ReplayPublished, not this helper.
func ReplayDefinition(def KnowledgeSet, pin ResolvedKnowledgeSet) (ResolvedKnowledgeSet, error) {
	if def.Retired {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset is retired")
	}
	if pin.SetID != "" && pin.SetID != def.SetID {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
	}
	if err := validateMountPaths(def.Sources); err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	if err := validateSourceCoordinates(def.Sources); err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	want := map[kernel.RepositoryID]kernel.CommitID{}
	for _, src := range def.Sources {
		commit, exists := pin.Repositories[src.Repository]
		if !exists {
			return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin membership does not match dataset")
		}
		if commit == "" || (src.Commit != "" && src.Commit != commit) {
			return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "pin commit is not the published source commit")
		}
		want[src.Repository] = commit
	}
	if !maps.Equal(want, pin.Repositories) || len(want) == 0 {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin membership does not match dataset")
	}
	items, err := datasetItemsFromSources(def.Sources, want)
	if err != nil {
		return ResolvedKnowledgeSet{}, err
	}
	id := HashResolved(def.SetID, def.Sources, want)
	if (pin.PinID != "" && pin.PinID != id) || (len(pin.Items) > 0 && !slices.Equal(pin.Items, items)) {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "pin scope does not match dataset")
	}
	if pin.Ref != "" && pin.Ref != DatasetVersionRef(def.Revision) {
		return ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "pin version does not match dataset")
	}
	return ResolvedKnowledgeSet{SetID: def.SetID, Revision: def.Revision, Ref: DatasetVersionRef(def.Revision), Repositories: want, Items: items, PinID: id}, nil
}

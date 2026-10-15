package datasets

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
	"sort"
)

// Preparation prepares a frozen Dataset basis through the selected provider.
type BasisPreparer interface {
	PrepareServingBasis(context.Context, objectmodel.Repository, kernel.CommitID) error
}
type Preparation struct {
	Enabled  bool
	Lookup   func(kernel.RepositoryID) (objectmodel.Repository, error)
	Provider BasisPreparer
	// Conflicts proves that the frozen candidate takes no Aspect of one object
	// from two source Repositories. It is required once a candidate has two
	// sources and does not depend on any plugin being configured.
	Conflicts func(dataset.ResolvedDataset) error
}

// Execute rejects a candidate whose range takes one Aspect of one object from
// two source Repositories, then prepares derived capabilities. The conflict
// check does not depend on any plugin being configured.
func (p Preparation) Execute(ctx context.Context, def dataset.Dataset) error {
	if err := p.checkHoldingConflicts(def); err != nil {
		return err
	}
	if !p.Enabled || p.Provider == nil {
		return nil
	}
	seen := map[kernel.RepositoryID]bool{}
	for _, source := range def.Sources {
		if seen[source.Repository] {
			continue
		}
		seen[source.Repository] = true
		// Repo configuration governs derived preparation. Installing an
		// extension on one source does not make basic publication of another
		// source depend on it. Unavailable configurations remain configured
		// and must return their original failure through the provider.
		if scoped, ok := p.Provider.(interface {
			ConfiguredFor(kernel.RepositoryID) bool
		}); ok && !scoped.ConfiguredFor(source.Repository) {
			continue
		}
		repo, err := p.Lookup(source.Repository)
		if err != nil {
			return err
		}
		if err := p.Provider.PrepareServingBasis(ctx, repo, source.Commit); err != nil {
			return err
		}
	}
	return nil
}

// checkHoldingConflicts hands the frozen candidate to the object conflict
// check. Object conflicts need at least two source Repositories.
func (p Preparation) checkHoldingConflicts(def dataset.Dataset) error {
	repositories := map[kernel.RepositoryID]kernel.CommitID{}
	for _, source := range def.Sources {
		repositories[source.Repository] = source.Commit
	}
	if len(repositories) < 2 {
		return nil
	}
	if p.Conflicts == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication cannot check object conflicts without a conflict check")
	}
	return p.Conflicts(dataset.ResolvedDataset{SetID: def.SetID, Revision: def.Revision, Repositories: repositories, Items: def.Items})
}

// ServingBasisDemand reads current accepted Dataset releases independently of
// source HEAD. The selected controller owns preparation and recovery attempts.
type ServingBasisDemand struct {
	Datasets dataset.Service
}

func (d ServingBasisDemand) Bases(repository kernel.RepositoryID) ([]kernel.CommitID, error) {
	seen := map[kernel.CommitID]bool{}
	bases := []kernel.CommitID{}
	if d.Datasets == nil {
		return bases, nil
	}
	for _, def := range d.Datasets.DumpState().Datasets {
		if def.Retired {
			continue
		}
		for _, source := range def.Sources {
			if source.Repository != repository || seen[source.Commit] {
				continue
			}
			if source.Commit == "" {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "accepted Dataset source has no fixed commit")
			}
			seen[source.Commit] = true
			bases = append(bases, source.Commit)
		}
	}
	sort.Slice(bases, func(i, j int) bool { return bases[i] < bases[j] })
	return bases, nil
}

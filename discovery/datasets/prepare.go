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
}

func (p Preparation) Execute(ctx context.Context, def dataset.KnowledgeSet) error {
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
	for _, def := range d.Datasets.DumpState().KnowledgeSets {
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

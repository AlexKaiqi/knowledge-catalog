package datasets

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
	"sort"
)

// BasisPreparer prepares a frozen Dataset basis through the selected provider.
type BasisPreparer interface {
	PrepareServingBasis(context.Context, objectmodel.Repository, kernel.CommitID) error
}
type Preparation struct {
	Enabled  bool
	Lookup   func(kernel.RepositoryID) (objectmodel.Repository, error)
	Provider BasisPreparer
	// PrepareDataset builds the whole frozen candidate. The composition root
	// adapts its Dataset resolution to the selected provider's reading basis.
	// Publication never calls it: acceptance is a metadata decision, and the
	// corpus of an accepted release is built afterwards, when the index
	// maintenance pass prepares that accepted revision (see
	// doc/design/dataset.md#发布).
	PrepareDataset func(context.Context, dataset.ResolvedDataset) error
	// Conflicts proves that the frozen candidate takes no Aspect of one object
	// from two source Repositories. It is required once a candidate has two
	// sources and does not depend on any plugin being configured.
	Conflicts func(dataset.ResolvedDataset) error
	// Dependencies freezes the versions of unselected Repositories holding
	// references the selected units make (doc/design/dataset.md#发布). Without
	// it a candidate cannot be published: an unchecked reference would only fail
	// later, at read time.
	Dependencies func(dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error)
}

// Execute is the publication half of preparation: it freezes reference
// dependencies and rejects a candidate whose range takes one Aspect of one
// object from two source Repositories. It returns the candidate with its
// dependencies; the sources it was given stay unchanged. It builds nothing, so
// its cost does not depend on how many objects the selected sources hold.
func (p Preparation) Execute(ctx context.Context, def dataset.Dataset) (dataset.Dataset, error) {
	if err := ctx.Err(); err != nil {
		return dataset.Dataset{}, err
	}
	return p.prepareDefinition(def)
}

// Build is the corpus half of preparation for one accepted candidate. It is
// the background task of an accepted release and the explicit retry path, and
// it is never part of publication. It does not change the candidate.
func (p Preparation) Build(ctx context.Context, def dataset.Dataset) error {
	if !p.Enabled {
		return ctx.Err()
	}
	candidate := dataset.ResolvedDataset{SetID: def.SetID, Revision: def.Revision, Repositories: frozenRepositories(def), Items: def.Items, Dependencies: def.Dependencies}
	if p.PrepareDataset != nil {
		return p.PrepareDataset(ctx, candidate)
	}
	if p.Provider == nil {
		return ctx.Err()
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
	return ctx.Err()
}

func (p Preparation) freezeDependencies(def dataset.Dataset) (dataset.Dataset, error) {
	if p.Dependencies == nil {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication cannot freeze reference dependencies without a dependency check")
	}
	dependencies, err := p.Dependencies(dataset.ResolvedDataset{SetID: def.SetID, Revision: def.Revision, Repositories: frozenRepositories(def), Items: def.Items})
	if err != nil {
		return dataset.Dataset{}, err
	}
	def.Dependencies = dependencies
	return def, nil
}

func frozenRepositories(def dataset.Dataset) map[kernel.RepositoryID]kernel.CommitID {
	repositories := map[kernel.RepositoryID]kernel.CommitID{}
	for _, source := range def.Sources {
		repositories[source.Repository] = source.Commit
	}
	return repositories
}

// checkHoldingConflicts hands the frozen candidate to the object conflict
// check. Object conflicts need at least two source Repositories.
func (p Preparation) checkHoldingConflicts(def dataset.Dataset) error {
	repositories := frozenRepositories(def)
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

func (p Preparation) prepareDefinition(def dataset.Dataset) (dataset.Dataset, error) {
	prepared, err := p.freezeDependencies(def)
	if err != nil {
		return dataset.Dataset{}, err
	}
	if err := p.checkHoldingConflicts(prepared); err != nil {
		return dataset.Dataset{}, err
	}
	return prepared, nil
}

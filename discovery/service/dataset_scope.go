package service

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type datasetCorpusProjection interface {
	datasetProjectionScope
	SearchDataset(context.Context, reader.MemberLookup, reader.DatasetPin, retrieval.SearchRequest) (retrieval.SearchResult, error)
}

func (e DatasetSearchExecutor) executeCorpus(ctx context.Context, request retrieval.SearchRequest, projection datasetCorpusProjection) (retrieval.SearchResult, error) {
	serving, logical, err := e.Resolve(ctx)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if serving == nil || logical == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset serving is unavailable")
	}
	queryContext := ports.WithCandidateRecheck(ctx, func(ctx context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, []objectmodel.UnitReadFailure, error) {
		return hydrateDatasetCorpusHit(ctx, serving, logical, hit)
	})
	result, err := projection.SearchDataset(queryContext, func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return e.Repositories.Require(id, kernel.ErrObjectRefUnresolved)
	}, serving.Pin(), request)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset %s revision %d cannot satisfy SEARCH: %v; an index declaration selected into the dataset must declare the required text/filter/sort use; see kc schema describe --dataset %s --revision %d", serving.Pin().SetID, serving.Pin().Revision, err, serving.Pin().SetID, serving.Pin().Revision)
		}
		return retrieval.SearchResult{}, err
	}
	for i, hit := range result.Hits {
		if len(hit.Version.Observations) == 0 {
			// The hit matched on fixed values; a delivered external unit that
			// failed is named beside the result, not shown as absent
			// (discovery#U1). The hit set itself stays as matched.
			var failures []objectmodel.UnitReadFailure
			hit, failures, err = hydrateDatasetCorpusHit(ctx, serving, logical, hit)
			if err != nil {
				return retrieval.SearchResult{}, err
			}
			for _, failure := range failures {
				result.Claims = append(result.Claims, retrieval.UnitFailureClaim(failure))
			}
		}
		hit, err = e.Deliver(ctx, hit)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		result.Hits[i] = hit
	}
	return result, nil
}

func hydrateDatasetCorpusHit(ctx context.Context, serving *reader.Serving, logical LogicalHydrator, hit retrieval.ObjectHit) (retrieval.ObjectHit, []objectmodel.UnitReadFailure, error) {
	sources := hit.Object.Sources
	if len(sources) == 0 {
		sources = []objectmodel.SourceBasis{{Repository: hit.Object.Repository, Commit: hit.Object.Commit}}
	}
	values := make([]objectmodel.ObjectValue, 0, len(sources))
	observations := []objectmodel.UnitObservation{}
	var failures []objectmodel.UnitReadFailure
	seen := map[kernel.RepositoryID]bool{}
	pin := serving.Pin()
	for _, source := range sources {
		if source.Repository == "" || source.Commit == "" || pin.Repositories[source.Repository] != source.Commit || seen[source.Repository] {
			return retrieval.ObjectHit{}, nil, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset hit source does not match its fixed revision")
		}
		seen[source.Repository] = true
		if err := ctx.Err(); err != nil {
			return retrieval.ObjectHit{}, nil, err
		}
		member, err := serving.Member(source.Repository)
		if err != nil {
			return retrieval.ObjectHit{}, nil, err
		}
		complete, err := serving.Contains(source.Repository, hit.Object.Address.ObjectID)
		if err != nil {
			return retrieval.ObjectHit{}, nil, err
		}
		observed, err := hydrateDatasetCorpusSource(ctx, member, logical, hit.Object.Address.ObjectID, source.Commit, complete)
		if err != nil {
			return retrieval.ObjectHit{}, nil, err
		}
		values = append(values, objectmodel.ObjectValue{Reference: observed.Reference, ObjectRef: observed.ObjectRef, Repository: observed.Repository, Commit: observed.Commit, Address: observed.Address, Value: observed.Value, Digest: observed.Digest, Provenance: observed.Provenance, Units: observed.Units, Declarations: observed.Declarations})
		observations = append(observations, observed.Observations...)
		failures = append(failures, observed.Failures...)
	}
	value, err := objectmodel.ComposeHoldingValues(hit.Object.Reference, values)
	if err != nil {
		return retrieval.ObjectHit{}, nil, err
	}
	value.Reference, value.Locator = hit.Object.Reference, hit.Object.Locator
	hit.Object = value
	hit.Version = retrieval.VersionOf(value)
	hit.Version.Observations = observations
	return hit, failures, nil
}

// Corpus hits and direct READ use the same logical root reference. Legacy
// hydrators that only accept complete objects retain their existing path.
func hydrateDatasetCorpusSource(ctx context.Context, member objectmodel.Repository, logical LogicalHydrator, id objectmodel.ObjectID, at kernel.CommitID, complete bool) (objectmodel.ObservedValue, error) {
	if references, ok := logical.(interface {
		ReadReference(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error)
	}); ok {
		selected, err := references.ReadReference(ctx, objectmodel.Reference{Repository: member.ID(), Object: id})
		if err != nil {
			return objectmodel.ObservedValue{}, err
		}
		if err := ctx.Err(); err != nil {
			return objectmodel.ObservedValue{}, err
		}
		if len(selected) != 1 || selected[0].ObjectID != id || selected[0].Locator != "" {
			return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset logical reader returned another root reference")
		}
		value := selected[0]
		if err := objectmodel.ValidateHydratedObject(member.ID(), at, id, objectmodel.ObjectValue{ObjectRef: value.ObjectRef, Repository: value.Repository, Commit: value.Commit, Address: value.Address, Units: value.Units, Declarations: value.Declarations}); err != nil {
			return objectmodel.ObservedValue{}, err
		}
		return value, nil
	}
	if !complete {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset logical reader cannot read selected object units")
	}
	value, err := member.Read(id, at)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	if err := objectmodel.ValidateHydratedObject(member.ID(), at, id, value); err != nil {
		return objectmodel.ObservedValue{}, err
	}
	return logical.Hydrate(ctx, objectmodel.FederatedValue{Reference: value.Reference, Locator: value.Locator, Digest: value.Digest, ObjectRef: value.ObjectRef, Repository: value.Repository, Commit: value.Commit, ObjectID: id, Address: value.Address, Value: value.Value, Provenance: value.Provenance, Units: value.Units, Declarations: value.Declarations}, nil)
}

type datasetProjectionScope interface {
	DatasetMember(reader.MemberLookup, reader.DatasetPin, kernel.RepositoryID) (objectmodel.Repository, error)
}

func datasetMember(projection any, lookup reader.MemberLookup, serving *reader.Serving, id kernel.RepositoryID) (objectmodel.Repository, error) {
	if scope, ok := projection.(datasetProjectionScope); ok {
		return scope.DatasetMember(lookup, serving.Pin(), id)
	}
	return serving.Member(id)
}

func datasetAccessContracts(projection any, lookup reader.MemberLookup, pin reader.DatasetPin) retrieval.AccessContractProvider {
	declarations, ok := projection.(accessContractProjection)
	if !ok {
		return nil
	}
	return func(repo objectmodel.Repository, at kernel.CommitID) (retrieval.AccessSpec, error) {
		if scope, ok := projection.(datasetProjectionScope); ok {
			selected, err := scope.DatasetMember(lookup, pin, repo.ID())
			if err != nil {
				return retrieval.AccessSpec{}, err
			}
			return declarations.AccessSpecAt(selected, at)
		}
		return declarations.AccessSpecAt(repo, at)
	}
}

func (r *datasetSearchRun) member(id kernel.RepositoryID) (objectmodel.Repository, error) {
	return datasetMember(r.exec.Projection, func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return r.exec.Repositories.Require(id, kernel.ErrObjectRefUnresolved)
	}, r.serving, id)
}

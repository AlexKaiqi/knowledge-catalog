package plugin

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// DatasetMember supplies the declaration namespace and engine identity of one
// frozen Dataset revision. Its canonical objects retain their source versions.
func (p *IndexPlugins) DatasetMember(lookup reader.MemberLookup, pin reader.DatasetPin, id kernel.RepositoryID) (objectmodel.Repository, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return nil, err
	}
	defer done()
	query, ok := instance.services.Queries.(interface {
		DatasetMember(reader.MemberLookup, reader.DatasetPin, kernel.RepositoryID) (objectmodel.Repository, error)
	})
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index instance cannot provide a Dataset declaration namespace")
	}
	return query.DatasetMember(lookup, pin, id)
}

func (p *IndexPlugins) SemanticDatasetWindow(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, request retrieval.SearchRequest, vector []float32) (retrieval.SearchResult, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer done()
	query, ok := instance.services.Queries.(interface {
		SemanticDatasetWindow(context.Context, reader.MemberLookup, reader.DatasetPin, retrieval.SearchRequest, []float32) (retrieval.SearchResult, error)
	})
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index instance cannot serve a Dataset semantic window")
	}
	return query.SemanticDatasetWindow(ctx, lookup, pin, request, vector)
}

// PrepareDatasetScope builds selected files, using only the declarations in
// this candidate. Preparation never reads the source's moving HEAD.
func (p *IndexPlugins) PrepareDatasetScope(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) error {
	ids := datasetRepositories(pin)
	if len(ids) > 0 {
		instance, done, err := p.takeDataset(pin)
		if err != nil {
			return err
		}
		defer done()
		if preparer, ok := instance.services.Queries.(interface {
			PrepareDataset(context.Context, reader.MemberLookup, reader.DatasetPin) (ports.IndexSync, error)
		}); ok {
			_, err := preparer.PrepareDataset(ctx, lookup, pin)
			return err
		}
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		member, err := p.DatasetMember(lookup, pin, id)
		if err != nil {
			return err
		}
		if _, err := p.EnsureAt(ctx, member, pin.Repositories[id]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// PrepareAcceptedDataset builds and adopts one accepted release's corpus. It is
// the explicit retry path of the release's own background build, not a second
// implementation: publication accepted metadata, so nothing has built this
// corpus yet. The instance that owns every source answers, which is the same
// instance the background lane would use.
func (p *IndexPlugins) PrepareAcceptedDataset(ctx context.Context, pin reader.DatasetPin) error {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return err
	}
	defer done()
	if instance.services.DatasetBuild == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "this index instance cannot build a Dataset corpus")
	}
	return instance.services.DatasetBuild(ctx, pin.SetID, pin.Revision)
}

// DatasetReadiness answers whether one accepted revision is searchable, without
// building or scanning. The index instance answers from the same metadata the
// query path checks, so the two cannot disagree.
func (p *IndexPlugins) DatasetReadiness(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.IndexReadiness, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return ports.IndexReadiness{}, err
	}
	defer done()
	query, ok := instance.services.Queries.(interface {
		DatasetReadiness(context.Context, reader.MemberLookup, reader.DatasetPin) (ports.IndexReadiness, error)
	})
	if !ok {
		return ports.IndexReadiness{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index instance cannot report Dataset readiness")
	}
	return query.DatasetReadiness(ctx, lookup, pin)
}

// IndexDiagnosis explains a readiness result. It is a separate call because it
// costs a full member reconciliation that the common question must not pay.
func (p *IndexPlugins) DatasetDiagnosis(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.IndexDiagnosis, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return ports.IndexDiagnosis{}, err
	}
	defer done()
	query, ok := instance.services.Queries.(interface {
		DatasetDiagnosis(context.Context, reader.MemberLookup, reader.DatasetPin) (ports.IndexDiagnosis, error)
	})
	if !ok {
		return ports.IndexDiagnosis{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index instance cannot diagnose Dataset readiness")
	}
	return query.DatasetDiagnosis(ctx, lookup, pin)
}

// RefreshPreparedDatasetState republishes the observation generation of a
// revision whose corpus just became ready. It is the second half of explicit
// preparation: the build makes the corpus exist, and the observation needs it
// to exist. The instance owning every source answers, as with the build.
//
// The runtime to observe with is the injected background one when this process
// has it, otherwise the caller's: an explicit refresh may observe its own
// source without configuring a worker (doc/design/discovery.md#观察).
func (p *IndexPlugins) RefreshPreparedDatasetState(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, caller ports.ObservationSource) (ports.StateSync, error) {
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return ports.StateSync{}, err
	}
	defer done()
	source := instance.services.Maintenance.ObservationSource()
	if source == nil {
		source = caller
	}
	if source == nil {
		return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset observations require a runtime")
	}
	query, ok := instance.services.Queries.(interface {
		RefreshDatasetState(context.Context, reader.MemberLookup, reader.DatasetPin, ports.ObservationSource) (ports.StateSync, error)
	})
	if !ok {
		return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index instance cannot refresh Dataset observations")
	}
	return query.RefreshDatasetState(ctx, lookup, pin, source)
}

// SearchDataset runs one corpus query on the published Dataset basis. The
// provider returns canonical values with their original source version vector.
func (p *IndexPlugins) SearchDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, request retrieval.SearchRequest) (retrieval.SearchResult, error) {
	ids := datasetRepositories(pin)
	if len(ids) == 0 {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid, "Dataset has no sources")
	}
	instance, done, err := p.takeDataset(pin)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer done()
	query, ok := instance.services.Queries.(interface {
		SearchDataset(context.Context, reader.MemberLookup, reader.DatasetPin, retrieval.SearchRequest) (retrieval.SearchResult, error)
	})
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index instance cannot serve a Dataset retrieval space")
	}
	request.Continuation, err = instance.unwrap(request.Continuation)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	result, err := query.SearchDataset(ctx, lookup, pin, request)
	if err == nil {
		result.Continuation = instance.wrap(result.Continuation)
	}
	return result, err
}

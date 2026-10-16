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
		if _, err := p.EnsureAt(member, pin.Repositories[id]); err != nil {
			return err
		}
	}
	return ctx.Err()
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

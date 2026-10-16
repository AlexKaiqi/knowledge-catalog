package plugin

import (
	"context"
	"errors"

	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// AfterSnapshot routes an accepted source change only to its configured
// contribution. Unconfigured sources retain independent fixed read/write.
func (p *IndexPlugins) AfterSnapshot(snapshot dataset.Snapshot) error {
	if p == nil || snapshot.Repository == nil {
		return nil
	}
	if !p.ConfiguredFor(snapshot.Repository.ID()) {
		return nil
	}
	return p.Desire(snapshot.Repository.ID(), snapshot.To)
}

func (p *IndexPlugins) CheckSearchProjectionAt(repo objectmodel.Repository, at kernel.CommitID) (index.Meta, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return index.Meta{}, err
	}
	defer done()
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		if queries, ok := instance.services.Queries.(interface {
			CheckDatasetProjection(reader.MemberLookup, reader.DatasetPin) (index.Meta, error)
		}); ok {
			lookup, pin := scoped.DatasetScope()
			return queries.CheckDatasetProjection(lookup, pin)
		}
	}
	return instance.services.Queries.CheckSearchProjectionAt(repo, at)
}

func (p *IndexPlugins) DescribeAt(repo objectmodel.Repository, at kernel.CommitID) (index.IndexDescriptor, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return index.IndexDescriptor{}, err
	}
	defer done()
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		if queries, ok := instance.services.Queries.(interface {
			DescribeDataset(reader.MemberLookup, reader.DatasetPin) (index.IndexDescriptor, error)
		}); ok {
			lookup, pin := scoped.DatasetScope()
			return queries.DescribeDataset(lookup, pin)
		}
	}
	return instance.services.Queries.DescribeAt(repo, at)
}

func (p *IndexPlugins) EnsureAt(repo objectmodel.Repository, at kernel.CommitID) (index.IndexSync, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return index.IndexSync{}, err
	}
	defer done()
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		if queries, ok := instance.services.Queries.(interface {
			PrepareDataset(context.Context, reader.MemberLookup, reader.DatasetPin) (index.IndexSync, error)
		}); ok {
			lookup, pin := scoped.DatasetScope()
			return queries.PrepareDataset(context.Background(), lookup, pin)
		}
	}
	return instance.services.Queries.EnsureAt(repo, at)
}

func (p *IndexPlugins) Ensure(repo objectmodel.Repository, at kernel.CommitID) (index.IndexSync, error) {
	if _, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		return p.EnsureAt(repo, at)
	}
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return index.IndexSync{}, err
	}
	defer done()
	return instance.services.Queries.Ensure(repo, at)
}

func (p *IndexPlugins) RefreshState(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID, source index.ObservationSource) (index.StateSync, error) {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return index.StateSync{}, err
	}
	defer done()
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		if queries, ok := instance.services.Queries.(interface {
			RefreshDatasetState(context.Context, reader.MemberLookup, reader.DatasetPin, index.ObservationSource) (index.StateSync, error)
		}); ok {
			lookup, pin := scoped.DatasetScope()
			return queries.RefreshDatasetState(ctx, lookup, pin, source)
		}
	}
	return instance.services.Queries.RefreshState(ctx, repo, at, source)
}

func (p *IndexPlugins) PrepareServingBasis(ctx context.Context, repo objectmodel.Repository, at kernel.CommitID) error {
	instance, done, err := p.takeRepository(repo)
	if err != nil {
		return err
	}
	defer done()
	return instance.services.Maintenance.PrepareServingBasis(ctx, repo, at)
}

func (p *IndexPlugins) Desire(repo kernel.RepositoryID, at kernel.CommitID) error {
	instance, done, err := p.take(repo)
	if err != nil {
		return err
	}
	defer done()
	return instance.services.Maintenance.Desire(repo, at)
}

func (p *IndexPlugins) Notify(notice index.ChangeNotice) error {
	instance, done, err := p.take(notice.Repository)
	if err != nil {
		return err
	}
	defer done()
	return instance.services.Maintenance.Notify(notice)
}

func (p *IndexPlugins) ApplyNotice(ctx context.Context, notice index.ChangeNotice, source index.ObservationSource) (index.StateSync, error) {
	instance, done, err := p.take(notice.Repository)
	if err != nil {
		return index.StateSync{}, err
	}
	defer done()
	return instance.services.Maintenance.ApplyNotice(ctx, notice, source)
}

func (p *IndexPlugins) CatchUp(ctx context.Context) error {
	done, err := p.takeAll()
	if err != nil {
		return err
	}
	defer done()
	var joined error
	// Instances run independently: one failed or blocked provider must not stop
	// another provider's maintenance. Each worker also retains its own ledger.
	results := make(chan error, len(p.instances))
	for _, instance := range p.instances {
		go func() { results <- instance.services.Maintenance.CatchUp(ctx) }()
	}
	for range p.instances {
		joined = errors.Join(joined, <-results)
	}
	return joined
}

// CatchUpRepository preserves the source's chosen control instance for a
// foreground maintenance task; unrelated instances do not join that task.
func (p *IndexPlugins) CatchUpRepository(ctx context.Context, repo kernel.RepositoryID) error {
	instance, done, err := p.take(repo)
	if err != nil {
		return err
	}
	defer done()
	return instance.services.Maintenance.CatchUpRepository(ctx, repo)
}

func (p *IndexPlugins) SetObservationSource(source index.ObservationSource) {
	done, err := p.takeAll()
	if err != nil {
		return
	}
	defer done()
	for _, instance := range p.instances {
		instance.services.Maintenance.SetObservationSource(source)
	}
}

func (p *IndexPlugins) Targets() ([]index.ProjectionTarget, error) {
	done, err := p.takeAll()
	if err != nil {
		return nil, err
	}
	defer done()
	var out []index.ProjectionTarget
	for _, instance := range p.instances {
		targets, err := instance.services.Maintenance.Targets()
		if err != nil {
			return nil, err
		}
		out = append(out, targets...)
	}
	return out, nil
}

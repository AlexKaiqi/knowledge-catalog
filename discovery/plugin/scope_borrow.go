package plugin

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func (p *IndexPlugins) takeRepository(repo objectmodel.Repository) (*indexInstance, func(), error) {
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		_, pin := scoped.DatasetScope()
		return p.takeDataset(pin)
	}
	return p.take(repo.ID())
}

// A corpus borrows one installed instance only after every fixed source is
// admitted. A foreign non-leading member must never reach the provider.
func (p *IndexPlugins) takeDataset(pin reader.DatasetPin) (*indexInstance, func(), error) {
	if p == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins are unavailable")
	}
	ids := datasetRepositories(pin)
	if len(ids) == 0 {
		return nil, nil, kernel.Fail(kernel.ErrUsageInvalid, "Dataset has no sources")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins have stopped")
	}
	var selected *indexInstance
	for _, id := range ids {
		instance := p.instanceForRepository(id)
		if instance == nil {
			return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "search projection is not available: no index plugin is installed for Repository %s", id)
		}
		if selected != nil && selected != instance {
			return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset sources require one installed index instance")
		}
		selected = instance
	}
	p.active.Add(1)
	return selected, p.active.Done, nil
}

func datasetRepositories(pin reader.DatasetPin) []kernel.RepositoryID {
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

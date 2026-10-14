package plugin

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

// IndexQueries retains the existing typed tasks and declaration witnesses.
// It is a consumption contract, not a directory of untyped plugin roles.
type IndexQueries interface {
	SearchProjection
	RelationProjection
	ProjectionDescriber
	ProjectionSynchronizer
	AccessSpecAt(objectmodel.Repository, kernel.CommitID) (retrieval.AccessSpec, error)
	CheckSearchProjectionAt(objectmodel.Repository, kernel.CommitID) (index.Meta, error)
}

type IndexMaintenance interface {
	ProjectionController
	PrepareServingBasis(context.Context, objectmodel.Repository, kernel.CommitID) error
	ApplyNotice(context.Context, index.ChangeNotice, index.ObservationSource) (index.StateSync, error)
	SetObservationSource(index.ObservationSource)
	Targets() ([]index.ProjectionTarget, error)
}

type IndexContribution struct {
	Queries     IndexQueries
	Maintenance IndexMaintenance
	Start       func(context.Context) error
	Stop        func()
}

type IndexPluginEntry struct {
	Affordance PluginAffordance
	Open       func(PluginScope) (IndexContribution, error)
}

type indexInstance struct {
	selection PluginSelection
	services  IndexContribution
	resources []func() error
	started   bool
	stopOnce  sync.Once
}

// IndexPlugins is the immutable running selection of each configured source.
// Opening and explanation never start workers. Activation and shutdown belong
// to the product host; borrowed task views do not own runtime resources.
type IndexPlugins struct {
	byRepo      map[kernel.RepositoryID]*indexInstance
	unavailable map[kernel.RepositoryID]error
	entries     map[string]IndexPluginEntry
	instances   []*indexInstance
	mu          sync.Mutex
	closed      bool
	started     bool
	stopped     bool
	closeOnce   sync.Once
	closeErr    error
	active      sync.WaitGroup
}

func OpenIndexPlugins(configs RepositoryPluginConfigs, entries map[string]IndexPluginEntry) (*IndexPlugins, error) {
	p := &IndexPlugins{byRepo: map[kernel.RepositoryID]*indexInstance{}, unavailable: map[kernel.RepositoryID]error{}, entries: map[string]IndexPluginEntry{}}
	for repo, err := range configs.Unavailable {
		if err == nil {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "unavailable Repository configuration requires its failure")
		}
		if _, exists := configs.Repositories[repo]; exists {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Repository configuration cannot be both available and unavailable")
		}
		p.unavailable[repo] = err
	}
	for name, entry := range entries {
		entry.Affordance.Tasks = append([]string(nil), entry.Affordance.Tasks...)
		p.entries[name] = entry
	}
	groups := map[PluginSelection][]kernel.RepositoryID{}
	for repo, config := range configs.Repositories {
		for _, selected := range config.Plugins {
			if _, ok := entries[selected.Name]; !ok {
				p.unavailable[repo] = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "configured plugin %s has no entry", selected.Name)
				break
			}
		}
		if p.unavailable[repo] != nil {
			continue
		}
		for _, selected := range config.Plugins {
			groups[selected] = append(groups[selected], repo)
		}
	}
	selections := make([]PluginSelection, 0, len(groups))
	for selected := range groups {
		selections = append(selections, selected)
	}
	sort.Slice(selections, func(i, j int) bool {
		if selections[i].Name != selections[j].Name {
			return selections[i].Name < selections[j].Name
		}
		return selections[i].Instance < selections[j].Instance
	})
	for _, selected := range selections {
		repos := groups[selected]
		sort.Slice(repos, func(i, j int) bool { return repos[i] < repos[j] })
		instance := &indexInstance{selection: selected}
		p.instances = append(p.instances, instance)
		entry := entries[selected.Name]
		if entry.Open == nil {
			return nil, errors.Join(kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin entry %s is incomplete", selected.Name), p.Close())
		}
		services, err := entry.Open(PluginScope{Name: selected.Name, Instance: selected.Instance, Repositories: append([]kernel.RepositoryID(nil), repos...),
			Own: func(close func() error) {
				if close != nil {
					instance.resources = append(instance.resources, close)
				}
			},
		})
		instance.services = services
		if err == nil && (nilContribution(services.Queries) || nilContribution(services.Maintenance) || services.Start == nil || services.Stop == nil) {
			err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s/%s has incomplete contributions", selected.Name, selected.Instance)
		}
		if err != nil {
			return nil, errors.Join(err, p.Close())
		}
		for _, repo := range repos {
			if p.byRepo[repo] != nil {
				return nil, errors.Join(kernel.Fail(kernel.ErrPreconditionFailed, "Repository has more than one index contribution"), p.Close())
			}
			p.byRepo[repo] = instance
		}
	}
	return p, nil
}

// An interface holding a nil implementation is not an executable contribution.
func nilContribution(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (p *IndexPlugins) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.closed || p.stopped {
		p.mu.Unlock()
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugins have stopped")
	}
	if p.started {
		p.mu.Unlock()
		return nil
	}
	for _, instance := range p.instances {
		instance.started = true
		if err := instance.services.Start(ctx); err != nil {
			p.mu.Unlock()
			return errors.Join(err, p.Close())
		}
	}
	p.started = true
	p.mu.Unlock()
	return nil
}

func (p *IndexPlugins) Configured() bool {
	return p != nil && (len(p.byRepo) != 0 || len(p.unavailable) != 0)
}

func (p *IndexPlugins) ConfiguredFor(repo kernel.RepositoryID) bool {
	return p != nil && (p.byRepo[repo] != nil || p.unavailable[repo] != nil)
}

func (p *IndexPlugins) Running() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started && !p.stopped && !p.closed
}

// Unactivated identifies the only state in which a root may finish assembly.
func (p *IndexPlugins) Unactivated() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.started && !p.stopped && !p.closed
}

func (p *IndexPlugins) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		p.StopBackground()
		// Stop cancels and joins background work without releasing query
		// resources. Cancellation must precede draining foreground work that
		// may be waiting for a Controller lock held by that background work.
		p.active.Wait()
		for i := len(p.instances) - 1; i >= 0; i-- {
			resources := p.instances[i].resources
			for j := len(resources) - 1; j >= 0; j-- {
				p.closeErr = errors.Join(p.closeErr, resources[j]())
			}
		}
	})
	return p.closeErr
}

// StopBackground cancels and joins workers without refusing calls or releasing
// query resources. A host drains accepted multi-step requests before Close.
func (p *IndexPlugins) StopBackground() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.stopped = true
	started := make([]*indexInstance, 0, len(p.instances))
	for _, instance := range p.instances {
		if instance.started {
			started = append(started, instance)
		}
	}
	p.mu.Unlock()
	for i := len(started) - 1; i >= 0; i-- {
		instance := started[i]
		instance.stopOnce.Do(instance.services.Stop)
	}
}

func (p *IndexPlugins) take(repo kernel.RepositoryID) (*indexInstance, func(), error) {
	if p == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins are unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins have stopped")
	}
	instance := p.byRepo[repo]
	if p.unavailable[repo] != nil {
		return nil, nil, p.unavailable[repo]
	}
	if instance == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "search projection is not available: Repository %s has no configured index plugin", repo)
	}
	p.active.Add(1)
	return instance, p.active.Done, nil
}

func (p *IndexPlugins) takeAll() (func(), error) {
	if p == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins are unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins have stopped")
	}
	p.active.Add(1)
	return p.active.Done, nil
}

func (p *IndexPlugins) selected(repo kernel.RepositoryID) (*indexInstance, error) {
	if p == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins are unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index plugins have stopped")
	}
	instance := p.byRepo[repo]
	if p.unavailable[repo] != nil {
		return nil, p.unavailable[repo]
	}
	if instance == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository %s has no configured index plugin", repo)
	}
	return instance, nil
}

func (p *IndexPlugins) Help(name string) (PluginAffordance, error) {
	if p == nil {
		return PluginAffordance{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin explanations are unavailable")
	}
	entry, ok := p.entries[name]
	if !ok || entry.Affordance.Name != name {
		return PluginAffordance{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s has no registered explanation", name)
	}
	out := entry.Affordance
	out.Tasks = append([]string(nil), out.Tasks...)
	return out, nil
}

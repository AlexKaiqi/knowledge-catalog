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
	// CatchUpRepository brings one interpreted source current without running
	// the maintenance of the instance's other sources.
	CatchUpRepository(context.Context, kernel.RepositoryID) error
	PrepareServingBasis(context.Context, objectmodel.Repository, kernel.CommitID) error
	ApplyNotice(context.Context, index.ChangeNotice, index.ObservationSource) (index.StateSync, error)
	SetObservationSource(index.ObservationSource)
	Targets() ([]index.ProjectionTarget, error)
}

type IndexContribution struct {
	Queries      IndexQueries
	Maintenance  IndexMaintenance
	Capabilities IndexCapabilities
	Start        func(context.Context) error
	Stop         func()
}

type IndexPluginEntry struct {
	Affordance PluginAffordance
	Open       func(PluginScope) (IndexContribution, error)
}

type indexInstance struct {
	selection PluginSelection
	catalog   string
	services  IndexContribution
	resources []func() error
	started   bool
	stopOnce  sync.Once
}

// IndexPlugins is the immutable running set of deployed index instances.
// Opening and explanation never start workers. Activation and shutdown belong
// to the product host; borrowed task views do not own runtime resources.
type IndexPlugins struct {
	byRepo             map[kernel.RepositoryID]*indexInstance
	containsRepository func(kernel.RepositoryID) bool
	entries            map[string]IndexPluginEntry
	instances          []*indexInstance
	mu                 sync.Mutex
	closed             bool
	started            bool
	stopped            bool
	closeOnce          sync.Once
	closeErr           error
	active             sync.WaitGroup
}

// OpenIndexPlugins opens the deployed instances of the compiled index entries.
// A catalog maps to exactly one instance. Until declarations route by their
// catalog (INDEX-SEARCH-SPACE-01), the one installed instance interprets every
// Repository of the deployment; more than one instance is refused, not guessed.
func OpenIndexPlugins(config PluginsConfig, scope PluginScope, entries map[string]IndexPluginEntry) (*IndexPlugins, error) {
	p := &IndexPlugins{byRepo: map[kernel.RepositoryID]*indexInstance{}, entries: map[string]IndexPluginEntry{}, containsRepository: scope.ContainsRepository}
	for name, entry := range entries {
		entry.Affordance.Tasks = append([]string(nil), entry.Affordance.Tasks...)
		p.entries[name] = entry
	}
	if err := config.CheckCatalogs(); err != nil {
		return nil, err
	}
	var selected []*indexInstance
	for _, name := range config.Names() {
		if _, ok := entries[name]; !ok {
			continue
		}
		plugin := config[name]
		for _, instanceName := range plugin.InstanceNames() {
			instance := plugin.Instances[instanceName]
			selection := PluginSelection{Name: name, Instance: instanceName}
			if instance.Catalog == "" || instance.Protocol != "" || len(instance.Runtimes) != 0 {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "index instance %s/%s requires exactly a catalog", name, instanceName)
			}
			selected = append(selected, &indexInstance{selection: selection, catalog: instance.Catalog})
		}
	}
	if len(selected) > 1 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "more than one index instance is not served until declarations route by catalog")
	}
	repos := append([]kernel.RepositoryID(nil), scope.Repositories...)
	sort.Slice(repos, func(i, j int) bool { return repos[i] < repos[j] })
	for _, instance := range selected {
		p.instances = append(p.instances, instance)
		entry := entries[instance.selection.Name]
		if entry.Open == nil {
			return nil, errors.Join(kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin entry %s is incomplete", instance.selection.Name), p.Close())
		}
		services, err := entry.Open(PluginScope{Name: instance.selection.Name, Instance: instance.selection.Instance, Repositories: append([]kernel.RepositoryID(nil), repos...),
			ContainsRepository: scope.ContainsRepository, RepositoryInventory: scope.RepositoryInventory,
			Own: func(close func() error) {
				if close != nil {
					instance.resources = append(instance.resources, close)
				}
			},
		})
		instance.services = services
		if err == nil && (nilContribution(services.Queries) || nilContribution(services.Maintenance) || services.Start == nil || services.Stop == nil) {
			err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s/%s has incomplete contributions", instance.selection.Name, instance.selection.Instance)
		}
		if err != nil {
			return nil, errors.Join(err, p.Close())
		}
		for _, repo := range repos {
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
	return p != nil && len(p.instances) != 0
}

func (p *IndexPlugins) ConfiguredFor(repo kernel.RepositoryID) bool {
	return p != nil && p.instanceForRepository(repo) != nil
}

// instanceForRepository uses host admission facts; the deployed interpreter is
// fixed even when the host adds or removes an admitted source.
func (p *IndexPlugins) instanceForRepository(repo kernel.RepositoryID) *indexInstance {
	if p.containsRepository != nil {
		if len(p.instances) == 1 && p.containsRepository(repo) {
			return p.instances[0]
		}
		return nil
	}
	return p.byRepo[repo]
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
	instance := p.instanceForRepository(repo)
	if instance == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "search projection is not available: no index plugin is installed for Repository %s", repo)
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
	instance := p.instanceForRepository(repo)
	if instance == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "no index plugin is installed for Repository %s", repo)
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

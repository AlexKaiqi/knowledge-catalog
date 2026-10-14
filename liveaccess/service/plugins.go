package service

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"

	common "kc/discovery/adoption"
	"kc/kernel"
	"kc/liveaccess"
	interfaces "kc/liveaccess/handler"
)

// PluginContribution contains the typed access ports actually supplied by one
// instance. Source operations and object interfaces are optional capabilities.
type PluginContribution struct {
	Lookup     liveaccess.StateLookup
	Accessor   liveaccess.ResourceOperationAccessor
	Interfaces *interfaces.InterfaceRegistry
}

type PluginEntry struct {
	Affordance common.PluginAffordance
	Open       func(common.PluginScope) (PluginContribution, error)
}

type pluginInstance struct {
	selection common.PluginSelection
	services  PluginContribution
	resources []func() error
}

// Plugins is the immutable Repo selection used by access consumers. It owns
// only access contributions, without index maintenance or a background worker.
type Plugins struct {
	byRepo      map[kernel.RepositoryID]*pluginInstance
	unavailable map[kernel.RepositoryID]error
	entries     map[string]PluginEntry
	instances   []*pluginInstance
	mu          sync.Mutex
	started     bool
	closed      bool
	active      sync.WaitGroup
	closeOnce   sync.Once
	closeErr    error
}

func OpenPlugins(configs common.RepositoryPluginConfigs, entries map[string]PluginEntry) (*Plugins, error) {
	p := &Plugins{byRepo: map[kernel.RepositoryID]*pluginInstance{}, unavailable: map[kernel.RepositoryID]error{}, entries: map[string]PluginEntry{}}
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
	groups := map[common.PluginSelection][]kernel.RepositoryID{}
	for repo, config := range configs.Repositories {
		for _, selection := range config.Plugins {
			if _, exists := entries[selection.Name]; !exists {
				p.unavailable[repo] = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "configured plugin %s has no entry", selection.Name)
				break
			}
		}
		if p.unavailable[repo] == nil {
			for _, selection := range config.Plugins {
				groups[selection] = append(groups[selection], repo)
			}
		}
	}
	selections := make([]common.PluginSelection, 0, len(groups))
	for selection := range groups {
		selections = append(selections, selection)
	}
	sort.Slice(selections, func(i, j int) bool {
		if selections[i].Name != selections[j].Name {
			return selections[i].Name < selections[j].Name
		}
		return selections[i].Instance < selections[j].Instance
	})
	for _, selection := range selections {
		repositories := groups[selection]
		sort.Slice(repositories, func(i, j int) bool { return repositories[i] < repositories[j] })
		instance := &pluginInstance{selection: selection}
		p.instances = append(p.instances, instance)
		entry := entries[selection.Name]
		if entry.Open == nil {
			p.failInstance(instance, repositories, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin entry %s is incomplete", selection.Name))
			continue
		}
		services, err := entry.Open(common.PluginScope{Name: selection.Name, Instance: selection.Instance, Repositories: append([]kernel.RepositoryID(nil), repositories...), Own: func(close func() error) {
			if close != nil {
				instance.resources = append(instance.resources, close)
			}
		}})
		instance.services = services
		if err == nil && missingPluginPort(services.Lookup) {
			err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s/%s has no State lookup contribution", selection.Name, selection.Instance)
		}
		if err != nil {
			p.failInstance(instance, repositories, err)
			continue
		}
		for _, repo := range repositories {
			if p.byRepo[repo] != nil {
				return nil, errors.Join(kernel.Fail(kernel.ErrPreconditionFailed, "Repository has more than one access contribution"), p.Close())
			}
			p.byRepo[repo] = instance
		}
	}
	return p, nil
}

func (p *Plugins) failInstance(instance *pluginInstance, repositories []kernel.RepositoryID, failure error) {
	for j := len(instance.resources) - 1; j >= 0; j-- {
		failure = errors.Join(failure, instance.resources[j]())
	}
	instance.resources = nil
	for _, repo := range repositories {
		p.unavailable[repo] = failure
	}
}

func missingPluginPort(value any) bool {
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

// Start activates the request-only selection; it does not start maintenance.
func (p *Plugins) Start(_ context.Context) error {
	if p == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access plugins are unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access plugins have stopped")
	}
	p.started = true
	return nil
}

func (p *Plugins) Unactivated() bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.started && !p.closed
}

func (p *Plugins) Configured() bool {
	return p != nil && (len(p.byRepo) != 0 || len(p.unavailable) != 0)
}

func (p *Plugins) ConfiguredFor(repo kernel.RepositoryID) bool {
	return p != nil && (p.byRepo[repo] != nil || p.unavailable[repo] != nil)
}

func (p *Plugins) take(repo kernel.RepositoryID) (*pluginInstance, func(), error) {
	if p == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access plugins are unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access plugins have stopped")
	}
	if err := p.unavailable[repo]; err != nil {
		return nil, nil, err
	}
	instance := p.byRepo[repo]
	if instance == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository %s has no configured access plugin", repo)
	}
	p.active.Add(1)
	return instance, p.active.Done, nil
}

// Close rejects new calls, drains accepted calls, then releases construction
// resources in reverse order exactly once, including after a partial Open.
func (p *Plugins) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
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

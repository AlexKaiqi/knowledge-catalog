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
	protocol  string
	runtimes  []string
	services  PluginContribution
	resources []func() error
	failure   error
}

// catalog is the LiveAccess capability name: a descriptor's runtime and
// protocol. An empty runtime is the protocol-wide instance.
type catalog struct{ runtime, protocol string }

// Plugins is the immutable set of deployed access instances. It owns only
// access contributions, without index maintenance or a background worker.
type Plugins struct {
	byCatalog map[catalog]*pluginInstance
	entries   map[string]PluginEntry
	instances []*pluginInstance
	mu        sync.Mutex
	started   bool
	closed    bool
	active    sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

// OpenPlugins opens the deployed instances of the compiled access entries.
// Each runtime and protocol maps to exactly one instance; an overlapping
// mapping is a deployment failure, never an assembly-order choice.
func OpenPlugins(config common.PluginsConfig, repositories []kernel.RepositoryID, entries map[string]PluginEntry) (*Plugins, error) {
	p := &Plugins{byCatalog: map[catalog]*pluginInstance{}, entries: map[string]PluginEntry{}}
	for name, entry := range entries {
		entry.Affordance.Tasks = append([]string(nil), entry.Affordance.Tasks...)
		p.entries[name] = entry
	}
	for _, name := range config.Names() {
		if _, ok := entries[name]; !ok {
			continue
		}
		plugin := config[name]
		for _, instanceName := range plugin.InstanceNames() {
			declared := plugin.Instances[instanceName]
			instance := &pluginInstance{selection: common.PluginSelection{Name: name, Instance: instanceName}, protocol: declared.Protocol, runtimes: append([]string(nil), declared.Runtimes...)}
			if declared.Protocol == "" || declared.Catalog != "" {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "access instance %s/%s requires a protocol and no catalog", name, instanceName)
			}
			keys := []catalog{{protocol: declared.Protocol}}
			if len(declared.Runtimes) != 0 {
				keys = keys[:0]
				for _, runtime := range declared.Runtimes {
					keys = append(keys, catalog{runtime: runtime, protocol: declared.Protocol})
				}
			}
			for _, key := range keys {
				if err := p.claim(key, instance); err != nil {
					return nil, err
				}
			}
			p.instances = append(p.instances, instance)
		}
	}
	repos := append([]kernel.RepositoryID(nil), repositories...)
	sort.Slice(repos, func(i, j int) bool { return repos[i] < repos[j] })
	for _, instance := range p.instances {
		entry := p.entries[instance.selection.Name]
		if entry.Open == nil {
			p.failInstance(instance, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin entry %s is incomplete", instance.selection.Name))
			continue
		}
		services, err := entry.Open(common.PluginScope{Name: instance.selection.Name, Instance: instance.selection.Instance, Repositories: append([]kernel.RepositoryID(nil), repos...), Own: func(close func() error) {
			if close != nil {
				instance.resources = append(instance.resources, close)
			}
		}})
		instance.services = services
		if err == nil && missingPluginPort(services.Lookup) {
			err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s/%s has no State lookup contribution", instance.selection.Name, instance.selection.Instance)
		}
		if err != nil {
			p.failInstance(instance, err)
		}
	}
	return p, nil
}

func (p *Plugins) claim(key catalog, instance *pluginInstance) error {
	for existing, prior := range p.byCatalog {
		if existing.protocol != key.protocol || prior == instance {
			continue
		}
		if existing.runtime == key.runtime || existing.runtime == "" || key.runtime == "" {
			return kernel.Fail(kernel.ErrPreconditionFailed, "protocol %s runtime %q maps to more than one instance: %s/%s and %s/%s", key.protocol, key.runtime, prior.selection.Name, prior.selection.Instance, instance.selection.Name, instance.selection.Instance)
		}
	}
	p.byCatalog[key] = instance
	return nil
}

// failInstance keeps the failure on its own catalog; other instances serve.
func (p *Plugins) failInstance(instance *pluginInstance, failure error) {
	for j := len(instance.resources) - 1; j >= 0; j-- {
		failure = errors.Join(failure, instance.resources[j]())
	}
	instance.resources = nil
	instance.failure = failure
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
	return p != nil && len(p.instances) != 0
}

func (p *Plugins) lookup(runtime, protocol string) *pluginInstance {
	if instance := p.byCatalog[catalog{runtime: runtime, protocol: protocol}]; instance != nil {
		return instance
	}
	return p.byCatalog[catalog{protocol: protocol}]
}

// take leases the instance deployed for one descriptor runtime and protocol.
func (p *Plugins) take(runtime, protocol string) (*pluginInstance, func(), error) {
	if p == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access plugins are unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access plugins have stopped")
	}
	instance := p.lookup(runtime, protocol)
	if instance == nil {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "no access instance is deployed for runtime %q protocol %q", runtime, protocol)
	}
	if instance.failure != nil {
		return nil, nil, instance.failure
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

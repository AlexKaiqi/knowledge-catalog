package service

import (
	"context"
	"sort"

	common "kc/discovery/adoption"
	"kc/interfaces"
	"kc/kernel"
	"kc/liveaccess"
	"kc/object/reader"
)

func (p *Plugins) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	instance, done, err := p.take(request.Binding.Repository)
	if err != nil {
		return liveaccess.StateObservation{}, err
	}
	defer done()
	observation, err := instance.services.Lookup.LookupState(ctx, request)
	if err != nil {
		return liveaccess.StateObservation{}, liveaccess.SafeFailure(ctx, err)
	}
	if err := liveaccess.CheckCredentialDisclosure(ctx, observation); err != nil {
		return liveaccess.StateObservation{}, err
	}
	return observation, nil
}

func (p *Plugins) AccessResource(ctx context.Context, request liveaccess.ResourceOperationRequest) (any, error) {
	instance, done, err := p.take(request.Descriptor.Repository)
	if err != nil {
		return nil, err
	}
	defer done()
	if missingPluginPort(instance.services.Accessor) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "configured access plugin has no source operation contribution")
	}
	value, err := instance.services.Accessor.AccessResource(ctx, request)
	if err != nil {
		return nil, liveaccess.SafeFailure(ctx, err)
	}
	if err := liveaccess.CheckCredentialDisclosure(ctx, value); err != nil {
		return nil, err
	}
	return value, nil
}

// InterfaceRegistryFor selects a handler using the source implementation's
// Repo and pinned contract. The private handler holds a lease throughout each
// actual validation/execution, so an escaped registry cannot outlive Close.
func (p *Plugins) InterfaceRegistryFor(implementation interfaces.ResolvedInterface) (*interfaces.InterfaceRegistry, error) {
	instance, done, err := p.take(implementation.Repository)
	if err != nil {
		return nil, err
	}
	defer done()
	registry := instance.services.Interfaces
	if missingPluginPort(instance.services.Accessor) {
		if registry == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "configured access plugin has no object interface contribution")
		}
	} else {
		registry, err = liveaccess.WithLiveAccess(registry, implementation, instance.services.Accessor)
		if err != nil {
			return nil, err
		}
	}
	registration, err := registry.Resolve(implementation)
	if err != nil {
		return nil, err
	}
	registration.Handler = &pluginInterfaceHandler{plugins: p, repository: implementation.Repository, instance: instance, handler: registration.Handler}
	private := interfaces.NewInterfaceRegistry()
	if err := private.Register(registration); err != nil {
		return nil, err
	}
	return private, nil
}

type pluginInterfaceHandler struct {
	plugins    *Plugins
	repository kernel.RepositoryID
	instance   *pluginInstance
	handler    interfaces.InterfaceHandler
}

func (h *pluginInterfaceHandler) take(repository kernel.RepositoryID) (func(), error) {
	if repository != h.repository {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Repository is outside this plugin interface scope")
	}
	instance, done, err := h.plugins.take(repository)
	if err != nil {
		return nil, err
	}
	if instance != h.instance {
		done()
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin interface selection has changed")
	}
	return done, nil
}

func (h *pluginInterfaceHandler) ValidateImplementation(ctx context.Context, implementation interfaces.ResolvedInterface) error {
	done, err := h.take(implementation.Repository)
	if err != nil {
		return err
	}
	defer done()
	return liveaccess.SafeFailure(ctx, h.handler.ValidateImplementation(ctx, implementation))
}

func (h *pluginInterfaceHandler) Execute(ctx context.Context, call interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
	done, err := h.take(call.Implementation.Repository)
	if err != nil {
		return interfaces.InterfaceResult{}, err
	}
	defer done()
	result, err := h.handler.Execute(ctx, call)
	if err != nil {
		return interfaces.InterfaceResult{}, liveaccess.SafeFailure(ctx, err)
	}
	if err := liveaccess.CheckCredentialDisclosure(ctx, result); err != nil {
		return interfaces.InterfaceResult{}, err
	}
	return result, nil
}

func (p *Plugins) Help(name string) (common.PluginAffordance, error) {
	if p == nil {
		return common.PluginAffordance{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin explanations are unavailable")
	}
	entry, exists := p.entries[name]
	if !exists || entry.Affordance.Name != name {
		return common.PluginAffordance{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s has no registered explanation", name)
	}
	affordance := entry.Affordance
	affordance.Tasks = append([]string(nil), affordance.Tasks...)
	return affordance, nil
}

// Overview reports only selection facts for the already authorized pin; it
// does not call a plugin, read Repo metadata, or expand the source scope.
func (p *Plugins) Overview(pin reader.DatasetPin) ([]common.SourcePlugin, error) {
	if p == nil {
		return nil, nil
	}
	repositories := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for repo := range pin.Repositories {
		repositories = append(repositories, repo)
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i] < repositories[j] })
	out := []common.SourcePlugin{}
	for _, repo := range repositories {
		if !p.ConfiguredFor(repo) {
			continue
		}
		instance, done, err := p.take(repo)
		if err != nil {
			return nil, err
		}
		selection := instance.selection
		done()
		files := []string{}
		for _, item := range pin.Items {
			if item.Repository == repo {
				file := item.File
				if file == "" {
					file = item.Prefix
				}
				files = append(files, file)
			}
		}
		sort.Strings(files)
		row := common.SourcePlugin{Repository: repo, Commit: pin.Repositories[repo], Files: files, Name: selection.Name, Instance: selection.Instance}
		if affordance, err := p.Help(selection.Name); err == nil {
			row.Summary, row.Help = affordance.Summary, "kc plugin "+selection.Name+" help"
		}
		out = append(out, row)
	}
	return out, nil
}

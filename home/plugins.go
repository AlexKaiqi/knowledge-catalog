package home

import (
	"kc/declarativeindex/opensearch"
	indexplugin "kc/declarativeindex/plugin"
	common "kc/discovery/adoption"
	indexplugins "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
)

// ExplainPlugin reads compiled registration facts without constructing a
// runtime or consulting deployment configuration. It never claims readiness.
func ExplainPlugin(name string) (indexplugins.PluginAffordance, error) {
	switch name {
	case "declarative-index":
		return indexplugin.Entry(indexplugin.Options{}).Affordance, nil
	case "liveaccess":
		return accessplugin.Entry(accessplugin.Options{}).Affordance, nil
	default:
		return indexplugins.PluginAffordance{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "plugin %s has no registered explanation", name)
	}
}

func (ws *Home) configureIndexPlugins(directory string) error {
	plugins, access, err := ws.openPlugins(directory)
	if err != nil {
		return err
	}
	ws.Index, ws.Plugins, ws.indexDirectory = plugins, plugins, directory
	ws.LiveAccess = access
	return nil
}

// SetLiveAccessEntries supplies compiled factories. The deployment's plugins
// configuration still decides which instances are installed.
func (ws *Home) SetLiveAccessEntries(entries map[string]liveservice.PluginEntry) error {
	if ws.readOnly || ws.borrowed || ws.pluginsVerified || !ws.Plugins.Unactivated() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "running plugin entries are immutable")
	}
	ws.liveAccessEntries = make(map[string]liveservice.PluginEntry, len(entries))
	for name, entry := range entries {
		ws.liveAccessEntries[name] = entry
	}
	return nil
}

// VerifyIndexPlugins is the root's one-time runtime assembly step. Cold
// connection management does not probe providers; serving reopens the
// deployed instances over the verified Repositories before activating workers.
// It is not a request-time refresh or a hot-loading operation.
func (ws *Home) VerifyIndexPlugins() error {
	if ws.readOnly || ws.borrowed {
		return kernel.Fail(kernel.ErrPreconditionFailed, "borrowed views cannot assemble plugins")
	}
	if !ws.Plugins.Unactivated() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "active or stopped plugins cannot be reassembled")
	}
	if ws.pluginsVerified {
		return nil
	}
	plugins, access, err := ws.openPlugins(ws.indexDirectory)
	if err != nil {
		return err
	}
	if err := ws.Plugins.Close(); err != nil {
		_ = plugins.Close()
		_ = access.Close()
		return err
	}
	if err := ws.LiveAccess.Close(); err != nil {
		_ = plugins.Close()
		_ = access.Close()
		return err
	}
	ws.Index, ws.Plugins = plugins, plugins
	ws.LiveAccess = access
	ws.DatasetPreparation.Enabled, ws.DatasetPreparation.Provider = plugins.Configured(), plugins
	ws.pluginsVerified = true
	return nil
}

// pluginsConfig is the deployment's installed plugins. Without an explicit
// plugins section the compiled defaults are installed; an explicit empty
// section installs none, so declarations stay ordinary content.
func (ws *Home) pluginsConfig() indexplugins.PluginsConfig {
	if ws.Stores.Plugins != nil {
		return *ws.Stores.Plugins
	}
	config := indexplugins.PluginsConfig{"liveaccess": {Instances: map[string]indexplugins.PluginInstanceConfig{"main": {Protocol: objectmodel.ProtocolResourceAccessV1}}}}
	if ws.Stores.Index != "none" {
		config["declarative-index"] = indexplugins.PluginConfig{Instances: map[string]indexplugins.PluginInstanceConfig{"main": {Catalog: indexplugin.Catalog}}}
	}
	return config
}

// indexMaintenanceInventory is the set of sources whose index cost the
// deployment accepted by registering them. A detached source keeps its
// content and history but leaves continuous projection maintenance, so its
// physical resources stay retired once removed (doc/design/discovery.md,
// 插件与作用域: 来源仓只提供内容).
func (ws *Home) indexMaintenanceInventory() []kernel.RepositoryID {
	ids := ws.Store.IDs()
	if ws.Datasets == nil {
		return ids
	}
	registered := ids[:0]
	for _, id := range ids {
		if ws.Datasets.HasRepository(id) {
			registered = append(registered, id)
		}
	}
	return registered
}

// openPlugins opens the deployed instances for every Repository of this
// deployment. No Repository or Dataset selects an instance.
func (ws *Home) openPlugins(directory string) (*indexplugins.IndexPlugins, *liveservice.Plugins, error) {
	repositories := ws.Store.IDs()
	models, err := opensearch.ConfiguredEmbeddingModels()
	if err != nil {
		return nil, nil, err
	}
	entry := indexplugin.Entry(indexplugin.Options{Directory: directory,
		RegisteredModels: models,
		Engine: func(namespace string) index.EngineOpener {
			return indexOpener(ws.File, instanceStores(ws.Stores, namespace))
		},
		Resources: func(namespace string) (index.ResourceCoordinator, error) {
			return indexResourceCoordinator(ws.File, instanceStores(ws.Stores, namespace))
		},
		Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return ws.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
		},
		Datasets: ws.Datasets, Hydrator: ws.Hydrator, DatasetPin: DatasetPin,
	})
	accessEntries := ws.liveAccessEntries
	if accessEntries == nil {
		accessEntries = map[string]liveservice.PluginEntry{"liveaccess": accessplugin.Entry(accessplugin.Options{})}
	}
	scopedEntries := make(map[string]liveservice.PluginEntry, len(accessEntries))
	for name, accessEntry := range accessEntries {
		if open := accessEntry.Open; open != nil {
			accessEntry.Open = func(scope common.PluginScope) (liveservice.PluginContribution, error) {
				scope.ContainsRepository = func(id kernel.RepositoryID) bool {
					_, ok := ws.Store.Get(id)
					return ok
				}
				return open(scope)
			}
		}
		scopedEntries[name] = accessEntry
	}
	known := []string{"declarative-index"}
	for name := range accessEntries {
		known = append(known, name)
	}
	config := ws.pluginsConfig()
	if err := config.Validate(known); err != nil {
		return nil, nil, err
	}
	plugins, err := indexplugins.OpenIndexPlugins(config, indexplugins.PluginScope{Repositories: repositories,
		ContainsRepository:  func(id kernel.RepositoryID) bool { _, ok := ws.Store.Get(id); return ok },
		RepositoryInventory: ws.indexMaintenanceInventory,
	}, map[string]indexplugins.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		return nil, nil, err
	}
	access, err := liveservice.OpenPlugins(config, repositories, scopedEntries)
	if err != nil {
		_ = plugins.Close()
		return nil, nil, err
	}
	return plugins, access, nil
}

package home

import (
	indexplugin "kc/declarativeindex/plugin"
	indexplugins "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
)

type repositoryMetadataReader interface {
	ReadRepositoryMetadata() (map[string]string, error)
}

// ExplainPlugin reads compiled registration facts without constructing a
// runtime or consulting Repository configuration. It never claims readiness.
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
	plugins, access, err := ws.openPlugins(directory, false)
	if err != nil {
		return err
	}
	ws.Index, ws.Plugins, ws.indexDirectory = plugins, plugins, directory
	ws.LiveAccess = access
	return nil
}

// SetLiveAccessEntries supplies compiled factories, never Repo adoption. The
// next root verification still derives every selection from Repo metadata.
func (ws *Home) SetLiveAccessEntries(entries map[string]liveservice.PluginEntry) error {
	if ws.readOnly || ws.pluginsVerified || !ws.Plugins.Unactivated() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "running plugin entries are immutable")
	}
	ws.liveAccessEntries = make(map[string]liveservice.PluginEntry, len(entries))
	for name, entry := range entries {
		ws.liveAccessEntries[name] = entry
	}
	return nil
}

// VerifyIndexPlugins is the root's one-time runtime assembly step. Cold
// connection management does not probe providers; serving verifies their
// Repository configuration before activating workers or accepting requests.
// It is not a request-time refresh or a hot-loading operation.
func (ws *Home) VerifyIndexPlugins() error {
	if ws.readOnly {
		return kernel.Fail(kernel.ErrPreconditionFailed, "borrowed views cannot assemble plugins")
	}
	if !ws.Plugins.Unactivated() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "active or stopped plugins cannot be reassembled")
	}
	if ws.pluginsVerified {
		return nil
	}
	plugins, access, err := ws.openPlugins(ws.indexDirectory, true)
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

func (ws *Home) openPlugins(directory string, verifyConnections bool) (*indexplugins.IndexPlugins, *liveservice.Plugins, error) {
	configs := indexplugins.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]indexplugins.RepositoryPluginConfig{}, Unavailable: map[kernel.RepositoryID]error{}}
	for _, id := range ws.Store.IDs() {
		store, ok := ws.Store.Get(id)
		if !ok {
			return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed, "configured Repository disappeared")
		}
		if _, connected := store.(*connectedSource); connected && !verifyConnections {
			configs.Unavailable[id] = kernel.Fail(kernel.ErrPreconditionFailed, "connected Repository plugin configuration has not been verified")
			continue
		}
		metadata := map[string]string{}
		if source, ok := store.(repositoryMetadataReader); ok {
			var err error
			metadata, err = source.ReadRepositoryMetadata()
			if err != nil {
				configs.Unavailable[id] = err
				continue
			}
		}
		config, err := indexplugins.ParseRepositoryPlugins(metadata)
		if err != nil {
			configs.Unavailable[id] = err
			continue
		}
		configs.Repositories[id] = config
	}
	entry := indexplugin.Entry(indexplugin.Options{Directory: directory,
		Engine: func(namespace string) index.EngineOpener {
			stores := ws.Stores
			if namespace != "" {
				if stores.OpenSearch.Namespace != "" {
					namespace = stores.OpenSearch.Namespace + "/" + namespace
				}
				stores.OpenSearch.Namespace = namespace
			}
			return indexOpener(ws.File, stores)
		},
		Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return ws.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
		},
		Datasets: ws.Datasets, Hydrator: ws.Hydrator,
	})
	accessEntries := ws.liveAccessEntries
	if accessEntries == nil {
		accessEntries = map[string]liveservice.PluginEntry{"liveaccess": accessplugin.Entry(accessplugin.Options{})}
	}
	known, accessNames := []string{"declarative-index"}, []string{}
	for name := range accessEntries {
		known = append(known, name)
		accessNames = append(accessNames, name)
	}
	plugins, err := indexplugins.OpenIndexPlugins(indexplugins.SelectRepositoryPlugins(configs, []string{"declarative-index"}, known), map[string]indexplugins.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		return nil, nil, err
	}
	access, err := liveservice.OpenPlugins(indexplugins.SelectRepositoryPlugins(configs, accessNames, known), accessEntries)
	if err != nil {
		_ = plugins.Close()
		return nil, nil, err
	}
	return plugins, access, nil
}

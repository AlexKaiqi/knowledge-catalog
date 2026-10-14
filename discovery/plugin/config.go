package plugin

import "kc/discovery/adoption"

// These aliases preserve the index consumer's public adoption vocabulary.
// Its definitions and interpretation are owned by the shared low-level contract.
const RepositoryPluginsKey = adoption.RepositoryPluginsKey

type PluginSelection = adoption.PluginSelection
type RepositoryPluginConfig = adoption.RepositoryPluginConfig
type RepositoryPluginConfigs = adoption.RepositoryPluginConfigs
type PluginAffordance = adoption.PluginAffordance
type SourcePlugin = adoption.SourcePlugin
type PluginScope = adoption.PluginScope

func ParseRepositoryPlugins(metadata map[string]string) (RepositoryPluginConfig, error) {
	return adoption.ParseRepositoryPlugins(metadata)
}

func SelectRepositoryPlugins(configs RepositoryPluginConfigs, consumed, known []string) RepositoryPluginConfigs {
	return adoption.SelectRepositoryPlugins(configs, consumed, known)
}

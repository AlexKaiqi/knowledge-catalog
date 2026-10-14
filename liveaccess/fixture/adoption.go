// Package fixture provides explicit LiveAccess adoption for component fixtures.
// It changes only native Repo metadata; it does not register runtime services.
package fixture

import (
	"encoding/json"

	common "kc/discovery/adoption"
	"kc/kernel"
	"kc/snapshot"
)

// MetadataSource is the native source metadata port used by explicit fixtures.
type MetadataSource interface {
	ReadRepositoryMetadata() (map[string]string, error)
	WriteRepositoryMetadata(map[string]string) error
}

// AdoptRepository adds the chosen access instance to one explicitly supplied
// component source, preserving other plugins and rejecting conflicting choices.
func AdoptRepository(repository kernel.RepositoryID, dsn, instance string) error {
	store, err := snapshot.OpenLakeFS(repository, dsn, "")
	if err != nil {
		return err
	}
	if closer, ok := store.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	metadata, ok := store.(MetadataSource)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess fixture requires native Repository metadata")
	}
	return AdoptMetadata(metadata, instance)
}

// AdoptMetadata selects one access instance without changing other choices.
func AdoptMetadata(metadata MetadataSource, instance string) error {
	properties, err := metadata.ReadRepositoryMetadata()
	if err != nil {
		return err
	}
	config, err := common.ParseRepositoryPlugins(properties)
	if err != nil {
		return err
	}
	for _, selected := range config.Plugins {
		if selected.Name == "liveaccess" {
			if selected.Instance != instance {
				return kernel.Fail(kernel.ErrPreconditionFailed, "LiveAccess fixture already selected another instance")
			}
			return nil
		}
	}
	config.Plugins = append(config.Plugins, common.PluginSelection{Name: "liveaccess", Instance: instance})
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if _, err := common.ParseRepositoryPlugins(map[string]string{common.RepositoryPluginsKey: string(raw)}); err != nil {
		return err
	}
	if properties == nil {
		properties = map[string]string{}
	}
	properties[common.RepositoryPluginsKey] = string(raw)
	return metadata.WriteRepositoryMetadata(properties)
}

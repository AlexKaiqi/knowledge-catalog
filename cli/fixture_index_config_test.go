package cli

import (
	"encoding/json"

	consumerplugin "kc/discovery/plugin"
	kchome "kc/home"
	"kc/kernel"
	"kc/snapshot"
)

// ConfigureFixtureIndexSource is an explicit input of object fixtures that
// already selected an index backend. Runtime assembly still reads native Repo
// metadata through production code; this helper registers no test services.
func ConfigureFixtureIndexSource(home string, id kernel.RepositoryID, dsn string) error {
	stores, err := kchome.ReadStores(home)
	if err != nil {
		return err
	}
	if stores.Index == "none" {
		return nil
	}
	source, err := snapshot.OpenLakeFS(id, dsn, "")
	if err != nil {
		return err
	}
	if closer, ok := source.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	metadata, ok := source.(interface {
		ReadRepositoryMetadata() (map[string]string, error)
		WriteRepositoryMetadata(map[string]string) error
	})
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "indexed object fixture requires native repository metadata")
	}
	properties, err := metadata.ReadRepositoryMetadata()
	if err != nil {
		return err
	}
	// Explicit choices, including an empty plugins array, remain test inputs.
	if _, exists := properties[consumerplugin.RepositoryPluginsKey]; exists {
		return nil
	}
	config, err := json.Marshal(consumerplugin.RepositoryPluginConfig{Version: 1,
		Plugins: []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "main"}},
	})
	if err != nil {
		return err
	}
	return metadata.WriteRepositoryMetadata(map[string]string{consumerplugin.RepositoryPluginsKey: string(config)})
}

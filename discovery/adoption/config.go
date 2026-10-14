package adoption

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"

	"kc/kernel"
)

// RepositoryPluginsKey is a native Repository metadata key. The value is
// interpreted here; Snapshot transports opaque metadata without plugin rules.
const RepositoryPluginsKey = "kc.plugins"

type PluginSelection struct {
	Name     string `json:"name"`
	Instance string `json:"instance"`
}

type RepositoryPluginConfig struct {
	Version int               `json:"version"`
	Plugins []PluginSelection `json:"plugins"`
}

// RepositoryPluginConfigs is one assembly-time read result, never persisted
// configuration. A failed source remains explicitly unavailable while its
// management recovery and independent sources can still be opened.
type RepositoryPluginConfigs struct {
	Repositories map[kernel.RepositoryID]RepositoryPluginConfig
	Unavailable  map[kernel.RepositoryID]error
}

var pluginIdentifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ParseRepositoryPlugins preserves missing configuration as no extensions.
// Malformed or unavailable metadata must never become an empty configuration.
func ParseRepositoryPlugins(metadata map[string]string) (RepositoryPluginConfig, error) {
	value, exists := metadata[RepositoryPluginsKey]
	if !exists {
		return RepositoryPluginConfig{Version: 1, Plugins: []PluginSelection{}}, nil
	}
	var config RepositoryPluginConfig
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, kernel.Fail(kernel.ErrPreconditionFailed, "Repository plugin configuration is invalid: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return config, kernel.Fail(kernel.ErrPreconditionFailed, "Repository plugin configuration must contain one document")
	}
	if config.Version != 1 || config.Plugins == nil {
		return config, kernel.Fail(kernel.ErrPreconditionFailed, "Repository plugin configuration requires version 1 and a plugins array")
	}
	seen := map[string]bool{}
	for _, selection := range config.Plugins {
		if !pluginIdentifier.MatchString(selection.Name) || !pluginIdentifier.MatchString(selection.Instance) || seen[selection.Name] {
			return config, kernel.Fail(kernel.ErrPreconditionFailed, "Repository plugin names and instances must be valid and names unique")
		}
		seen[selection.Name] = true
	}
	return config, nil
}

type PluginAffordance struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary"`
	Usage   string   `json:"usage"`
	Tasks   []string `json:"tasks"`
}

type SourcePlugin struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Files      []string            `json:"files"`
	Name       string              `json:"name"`
	Instance   string              `json:"instance"`
	Summary    string              `json:"summary,omitempty"`
	Help       string              `json:"help,omitempty"`
}

// SelectRepositoryPlugins partitions one authoritative configuration for a
// typed consumer. Known entries of another capability do not enable this one;
// a missing compiled entry remains an explicit source configuration failure.
func SelectRepositoryPlugins(configs RepositoryPluginConfigs, consumed, known []string) RepositoryPluginConfigs {
	accepted, installed := map[string]bool{}, map[string]bool{}
	for _, name := range consumed {
		accepted[name] = true
	}
	for _, name := range known {
		installed[name] = true
	}
	out := RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]RepositoryPluginConfig{}, Unavailable: map[kernel.RepositoryID]error{}}
	for repo, err := range configs.Unavailable {
		out.Unavailable[repo] = err
	}
	for repo, config := range configs.Repositories {
		selected := RepositoryPluginConfig{Version: config.Version, Plugins: []PluginSelection{}}
		for _, entry := range config.Plugins {
			if !installed[entry.Name] {
				out.Unavailable[repo] = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "configured plugin %s has no entry", entry.Name)
				break
			}
			if accepted[entry.Name] {
				selected.Plugins = append(selected.Plugins, entry)
			}
		}
		if out.Unavailable[repo] == nil {
			out.Repositories[repo] = selected
		}
	}
	return out
}

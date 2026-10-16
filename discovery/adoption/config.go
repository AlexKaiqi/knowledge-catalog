package adoption

import (
	"regexp"
	"sort"

	"kc/kernel"
)

// PluginSelection names one deployed plugin instance. It identifies who
// interprets a declaration; it is never chosen by a Repository or Dataset.
type PluginSelection struct {
	Name     string `json:"name"`
	Instance string `json:"instance"`
}

// PluginInstanceConfig is one deployed instance. A declarative index instance
// answers one capability catalog; a LiveAccess instance answers one protocol
// for the listed runtimes (all runtimes of that protocol when none are listed).
type PluginInstanceConfig struct {
	Catalog  string   `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Protocol string   `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Runtimes []string `json:"runtimes,omitempty" yaml:"runtimes,omitempty"`
}

type PluginConfig struct {
	Instances map[string]PluginInstanceConfig `json:"instances" yaml:"instances"`
}

// PluginsConfig is the deployment's installed plugins keyed by plugin name.
// A plugin absent here is not installed: its declarations stay ordinary
// content and requests report the missing capability.
type PluginsConfig map[string]PluginConfig

var pluginIdentifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Validate checks names against the compiled plugins. Each consumer checks
// its own catalog mapping, including that one catalog has one instance.
func (c PluginsConfig) Validate(known []string) error {
	installed := map[string]bool{}
	for _, name := range known {
		installed[name] = true
	}
	for _, name := range c.Names() {
		if !pluginIdentifier.MatchString(name) || !installed[name] {
			return kernel.Fail(kernel.ErrPreconditionFailed, "deployment installs unknown plugin %q", name)
		}
		if len(c[name].Instances) == 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "deployment plugin %s requires at least one instance", name)
		}
		for instance := range c[name].Instances {
			if !pluginIdentifier.MatchString(instance) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "deployment plugin %s has invalid instance name %q", name, instance)
			}
		}
	}
	return c.CheckCatalogs()
}

// CheckCatalogs refuses a deployment that maps one catalog to more than one
// instance; the interpreter of a declaration is never guessed.
func (c PluginsConfig) CheckCatalogs() error {
	owners := map[string]PluginSelection{}
	for _, name := range c.Names() {
		plugin := c[name]
		for _, instance := range plugin.InstanceNames() {
			catalog := plugin.Instances[instance].Catalog
			if catalog == "" {
				continue
			}
			if prior, exists := owners[catalog]; exists {
				return kernel.Fail(kernel.ErrPreconditionFailed, "catalog %s maps to more than one instance: %s/%s and %s/%s", catalog, prior.Name, prior.Instance, name, instance)
			}
			owners[catalog] = PluginSelection{Name: name, Instance: instance}
		}
	}
	return nil
}

func (c PluginsConfig) Names() []string {
	names := make([]string, 0, len(c))
	for name := range c {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// InstanceNames returns one plugin's instances in a stable order.
func (c PluginConfig) InstanceNames() []string {
	names := make([]string, 0, len(c.Instances))
	for name := range c.Instances {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	Catalog    string              `json:"catalog,omitempty"`
	Summary    string              `json:"summary,omitempty"`
	Help       string              `json:"help,omitempty"`
}

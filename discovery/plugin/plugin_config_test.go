package plugin

import (
	"strings"
	"testing"

	"kc/kernel"
)

func indexInstances(instances map[string]PluginInstanceConfig) PluginsConfig {
	return PluginsConfig{"declarative-index": {Instances: instances}}
}

func TestIndexInstancesMapEachCatalogToOneInstance(t *testing.T) {
	entries := map[string]IndexPluginEntry{"declarative-index": {Affordance: PluginAffordance{Name: "declarative-index"}}}
	repos := []kernel.RepositoryID{"orders", "sales"}
	_, err := OpenIndexPlugins(indexInstances(map[string]PluginInstanceConfig{
		"opensearch-a": {Catalog: "declarative-index/opensearch@2026-10"},
		"opensearch-b": {Catalog: "declarative-index/opensearch@2026-10"},
	}), PluginScope{Repositories: repos}, entries)
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !strings.Contains(err.Error(), "declarative-index/opensearch@2026-10") ||
		!strings.Contains(err.Error(), "opensearch-a") || !strings.Contains(err.Error(), "opensearch-b") {
		t.Fatalf("duplicate catalog mapping was not refused with both instances: %v", err)
	}
	for name, instance := range map[string]PluginInstanceConfig{
		"missing catalog":   {},
		"access-only shape": {Catalog: "declarative-index/opensearch@2026-10", Protocol: "resource-access/v1"},
	} {
		if _, err := OpenIndexPlugins(indexInstances(map[string]PluginInstanceConfig{"main": instance}), PluginScope{Repositories: repos}, entries); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("%s opened an index instance: %v", name, err)
		}
	}
	if _, err := OpenIndexPlugins(indexInstances(map[string]PluginInstanceConfig{
		"opensearch": {Catalog: "declarative-index/opensearch@2026-10"},
		"elastic":    {Catalog: "declarative-index/elastic@2026-01"},
	}), PluginScope{Repositories: repos}, entries); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("several instances were served before declarations route by catalog: %v", err)
	}
}

func TestIndexPluginAbsentReportsMissingCapability(t *testing.T) {
	entries := map[string]IndexPluginEntry{"declarative-index": {Affordance: PluginAffordance{Name: "declarative-index"}}}
	plugins, err := OpenIndexPlugins(PluginsConfig{}, PluginScope{Repositories: []kernel.RepositoryID{"orders"}}, entries)
	if err != nil {
		t.Fatal(err)
	}
	defer plugins.Close()
	if plugins.Configured() || plugins.ConfiguredFor("orders") {
		t.Fatal("an absent plugin was reported as installed")
	}
	if _, _, err := plugins.take("orders"); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("absent index plugin did not report a missing capability: %v", err)
	}
	if _, err := plugins.Help("declarative-index"); err != nil {
		t.Fatalf("compiled explanation depends on installation: %v", err)
	}
}

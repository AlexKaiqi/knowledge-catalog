package home

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	indexplugin "kc/declarativeindex/plugin"
	consumerplugin "kc/discovery/plugin"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"

	"gopkg.in/yaml.v3"
)

// writeDeploymentDocument writes cfg as YAML after edit adjusts its document,
// so retired shapes reach the real strict reader.
func writeDeploymentDocument(t *testing.T, cfg DeploymentConfig, edit func(map[string]any)) string {
	t.Helper()
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	edit(document)
	if raw, err = yaml.Marshal(document); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deployment.yaml")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDeploymentRefusesRetiredPerRepositoryPluginSelection(t *testing.T) {
	path := writeDeploymentDocument(t, deploymentFixture(t), func(document map[string]any) {
		repositories := document["repositories"].([]any)
		repositories[0].(map[string]any)["plugins"] = map[string]any{"liveaccess": "live-main", "declarative-index": "opensearch-main"}
	})
	if _, err := ReadDeployment(path); kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), "per-Repository plugin selection is retired") {
		t.Fatalf("retired per-Repository selection was not refused for its own reason: %v", err)
	}
}

func TestDeploymentRefusesAmbiguousOrUnknownPluginInstances(t *testing.T) {
	for name, row := range map[string]struct {
		plugins consumerplugin.PluginsConfig
		want    string
	}{
		"duplicate-catalog": {consumerplugin.PluginsConfig{"declarative-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{
			"opensearch-a": {Catalog: indexplugin.Catalog}, "opensearch-b": {Catalog: indexplugin.Catalog},
		}}}, "opensearch-a and declarative-index/opensearch-b"},
		"unknown-plugin": {consumerplugin.PluginsConfig{"unregistered": {Instances: map[string]consumerplugin.PluginInstanceConfig{"main": {Catalog: "x/v1"}}}}, "unregistered"},
		"no-instance":    {consumerplugin.PluginsConfig{"liveaccess": {}}, "at least one instance"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := deploymentFixture(t)
			cfg.Plugins = &row.plugins
			path := writeDeploymentDocument(t, cfg, func(map[string]any) {})
			if _, err := ReadDeployment(path); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !strings.Contains(err.Error(), row.want) {
				t.Fatalf("deployment started with an ambiguous plugin installation: %v", err)
			}
		})
	}
	cfg := deploymentFixture(t)
	cfg.Stores.Plugins = &consumerplugin.PluginsConfig{}
	if err := cfg.Validate(); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("plugins under stores were accepted: %v", err)
	}
}

func openPluginDeployment(t *testing.T, plugins *consumerplugin.PluginsConfig) (*Home, error) {
	t.Helper()
	cfg := deploymentFixture(t)
	cfg.Stores.Index = "none"
	cfg.Plugins = plugins
	if err := InitializeDeployment(cfg, func(dir, _ string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if ws != nil {
		t.Cleanup(func() { _ = ws.Close() })
	}
	return ws, err
}

func liveLookup(runtime string) liveaccess.StateLookupRequest {
	return liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: objectmodel.SystemRepositoryID, Runtime: runtime, Protocol: objectmodel.ProtocolResourceAccessV1}}
}

func TestDeploymentWithoutPluginInstancesExplainsMissingCapability(t *testing.T) {
	ws, err := openPluginDeployment(t, &consumerplugin.PluginsConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Plugins.Configured() {
		t.Fatal("an explicit empty plugins section installed an index instance")
	}
	if _, err := ws.LiveAccess.LookupState(context.Background(), liveLookup("http")); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !strings.Contains(err.Error(), "no access instance is deployed") {
		t.Fatalf("value without an installed interpreter did not name the missing capability: %v", err)
	}
}

func TestDeploymentRefusesOverlappingAccessRuntimes(t *testing.T) {
	plugins := consumerplugin.PluginsConfig{"liveaccess": {Instances: map[string]consumerplugin.PluginInstanceConfig{
		"wide": {Protocol: objectmodel.ProtocolResourceAccessV1}, "listed": {Protocol: objectmodel.ProtocolResourceAccessV1, Runtimes: []string{"http"}},
	}}}
	if _, err := openPluginDeployment(t, &plugins); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !strings.Contains(err.Error(), "wide") || !strings.Contains(err.Error(), "listed") {
		t.Fatalf("deployment started with one access catalog mapped to two instances: %v", err)
	}
}

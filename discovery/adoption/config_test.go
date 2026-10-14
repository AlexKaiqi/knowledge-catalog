package adoption_test

import (
	"reflect"
	"testing"

	"kc/discovery/adoption"
	legacy "kc/discovery/plugin"
	"kc/kernel"
)

func TestLegacyPluginAdoptionTypesKeepIdentityAndShape(t *testing.T) {
	for _, pair := range [][2]any{
		{adoption.PluginSelection{}, legacy.PluginSelection{}},
		{adoption.RepositoryPluginConfig{}, legacy.RepositoryPluginConfig{}},
		{adoption.RepositoryPluginConfigs{}, legacy.RepositoryPluginConfigs{}},
		{adoption.PluginScope{}, legacy.PluginScope{}},
		{adoption.PluginAffordance{}, legacy.PluginAffordance{}},
		{adoption.SourcePlugin{}, legacy.SourcePlugin{}},
	} {
		if reflect.TypeOf(pair[0]) != reflect.TypeOf(pair[1]) {
			t.Fatalf("legacy contract became a copied named type: %T versus %T", pair[0], pair[1])
		}
	}
	if adoption.RepositoryPluginsKey != "kc.plugins" {
		t.Fatal("Repository metadata key changed")
	}
	if adoption.RepositoryPluginsKey != legacy.RepositoryPluginsKey {
		t.Fatal("legacy metadata key changed")
	}
	for _, raw := range []string{`{"version":1,"plugins":[{"name":"liveaccess","instance":"source"}]}`, `{"version":1,"plugins":null}`} {
		metadata := map[string]string{adoption.RepositoryPluginsKey: raw}
		current, currentErr := adoption.ParseRepositoryPlugins(metadata)
		prior, priorErr := legacy.ParseRepositoryPlugins(metadata)
		if !reflect.DeepEqual(current, prior) || !reflect.DeepEqual(kernel.Normalize(currentErr), kernel.Normalize(priorErr)) {
			t.Fatalf("legacy parser changed result or source error: %#v %v / %#v %v", current, currentErr, prior, priorErr)
		}
	}
}

func TestSelectRepositoryPluginsPreservesTypedAdoptionAndSourceFaults(t *testing.T) {
	failure := kernel.Fail(kernel.ErrTemporaryUnavailable, "source metadata unavailable")
	configs := adoption.RepositoryPluginConfigs{
		Repositories: map[kernel.RepositoryID]adoption.RepositoryPluginConfig{
			"mixed":      {Version: 1, Plugins: []adoption.PluginSelection{{Name: "declarative-index", Instance: "index"}, {Name: "liveaccess", Instance: "access"}}},
			"index-only": {Version: 1, Plugins: []adoption.PluginSelection{{Name: "declarative-index", Instance: "index"}}},
			"disabled":   {Version: 1, Plugins: []adoption.PluginSelection{}},
			"unknown":    {Version: 1, Plugins: []adoption.PluginSelection{{Name: "missing", Instance: "source"}}},
		},
		Unavailable: map[kernel.RepositoryID]error{"failed": failure},
	}
	consumed, known := []string{"liveaccess"}, []string{"liveaccess", "declarative-index"}
	selected := adoption.SelectRepositoryPlugins(configs, consumed, known)
	if !reflect.DeepEqual(selected, legacy.SelectRepositoryPlugins(configs, consumed, known)) {
		t.Fatal("compatibility partition changed adopted source facts")
	}
	if !reflect.DeepEqual(selected.Repositories["mixed"].Plugins, []adoption.PluginSelection{{Name: "liveaccess", Instance: "access"}}) ||
		len(selected.Repositories["index-only"].Plugins) != 0 || len(selected.Repositories["disabled"].Plugins) != 0 {
		t.Fatalf("another capability enabled access or replaced its choice: %#v", selected.Repositories)
	}
	if _, admitted := selected.Repositories["unknown"]; admitted || kernel.CodeOf(selected.Unavailable["unknown"]) != kernel.ErrCapabilityUnsatisfied || selected.Unavailable["failed"] != failure {
		t.Fatalf("source configuration failure became absent configuration: %#v", selected)
	}
	selected.Repositories["mixed"].Plugins[0].Instance = "caller mutation"
	delete(selected.Unavailable, "failed")
	if configs.Repositories["mixed"].Plugins[1].Instance != "access" || configs.Unavailable["failed"] != failure {
		t.Fatal("one typed consumer mutated the common adopted input")
	}
}

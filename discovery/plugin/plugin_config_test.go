package plugin

import (
	"reflect"
	"testing"

	"kc/kernel"
)

func TestRepositoryPluginConfigurationDistinguishesMissingDisabledAndInvalid(t *testing.T) {
	for _, metadata := range []map[string]string{nil, {}, {RepositoryPluginsKey: `{"version":1,"plugins":[]}`}} {
		config, err := ParseRepositoryPlugins(metadata)
		if err != nil || config.Version != 1 || !reflect.DeepEqual(config.Plugins, []PluginSelection{}) {
			t.Fatalf("empty config changed: %+v %v", config, err)
		}
	}
	config, err := ParseRepositoryPlugins(map[string]string{RepositoryPluginsKey: `{"version":1,"plugins":[{"name":"declarative-index","instance":"orders"}]}`, "description": "ordinary metadata"})
	if err != nil || !reflect.DeepEqual(config.Plugins, []PluginSelection{{Name: "declarative-index", Instance: "orders"}}) {
		t.Fatalf("selection lost: %+v %v", config, err)
	}
	for _, raw := range []string{"", "null", `{"version":1}`, `{"version":1,"plugins":null}`, `{"version":2,"plugins":[]}`, `{"version":1,"plugins":[],"default":"hidden"}`, `{"version":1,"plugins":[{"name":"declarative-index","instance":"../private"}]}`, `{"version":1,"plugins":[{"name":"declarative-index","instance":"a"},{"name":"declarative-index","instance":"b"}]}`, `{"version":1,"plugins":[]} {}`} {
		if _, err := ParseRepositoryPlugins(map[string]string{RepositoryPluginsKey: raw}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("invalid config became empty/enabled: %q %v", raw, err)
		}
	}
}

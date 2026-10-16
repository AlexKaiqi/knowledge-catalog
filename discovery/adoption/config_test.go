package adoption_test

import (
	"reflect"
	"testing"

	"kc/discovery/adoption"
	legacy "kc/discovery/plugin"
	"kc/kernel"
)

func TestPluginAdoptionAliasesKeepIdentityAndShape(t *testing.T) {
	for _, pair := range [][2]any{
		{adoption.PluginSelection{}, legacy.PluginSelection{}},
		{adoption.PluginsConfig{}, legacy.PluginsConfig{}},
		{adoption.PluginConfig{}, legacy.PluginConfig{}},
		{adoption.PluginInstanceConfig{}, legacy.PluginInstanceConfig{}},
		{adoption.PluginScope{}, legacy.PluginScope{}},
		{adoption.PluginAffordance{}, legacy.PluginAffordance{}},
		{adoption.SourcePlugin{}, legacy.SourcePlugin{}},
	} {
		if reflect.TypeOf(pair[0]) != reflect.TypeOf(pair[1]) {
			t.Fatalf("shared contract became a copied named type: %T versus %T", pair[0], pair[1])
		}
	}
}

func TestDeploymentPluginsAcceptOnlyCompiledNamesWithInstances(t *testing.T) {
	known := []string{"declarative-index", "liveaccess"}
	valid := adoption.PluginsConfig{
		"declarative-index": {Instances: map[string]adoption.PluginInstanceConfig{"main": {Catalog: "declarative-index/opensearch@2026-10"}}},
		"liveaccess":        {Instances: map[string]adoption.PluginInstanceConfig{"main": {Protocol: "resource-access/v1"}}},
	}
	if err := valid.Validate(known); err != nil {
		t.Fatalf("deployed instances were rejected: %v", err)
	}
	if err := (adoption.PluginsConfig{}).Validate(known); err != nil {
		t.Fatalf("installing no plugins was rejected: %v", err)
	}
	for name, config := range map[string]adoption.PluginsConfig{
		"unknown plugin":   {"elastic-index": {Instances: map[string]adoption.PluginInstanceConfig{"main": {Catalog: "x"}}}},
		"no instances":     {"liveaccess": {}},
		"invalid instance": {"liveaccess": {Instances: map[string]adoption.PluginInstanceConfig{"../main": {Protocol: "resource-access/v1"}}}},
	} {
		if err := config.Validate(known); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("%s was accepted as a deployment: %v", name, err)
		}
	}
}

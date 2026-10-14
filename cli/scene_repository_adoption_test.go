package cli_test

import (
	"testing"

	"kc/discovery/adoption"
	"kc/kernel"
	"kc/liveaccess/fixture"
	"kc/snapshot"
)

func TestSceneRepositoryFixtureRequiresExplicitAccessSelection(t *testing.T) {
	for _, test := range []struct {
		given, instance string
		invalid         bool
	}{
		{"existing repository kr://scene/knowledge", "", false},
		{"existing repository kr://scene/knowledge with LiveAccess instance main", "main", false},
		{"existing repository kr://scene/knowledge with LiveAccess instance", "", true},
		{"existing repository kr://scene/knowledge with LiveAccess instance main extra", "", true},
	} {
		t.Run(test.given, func(t *testing.T) {
			feature, err := parseSceneFeature("Feature: source\nScenario: fixture\nGiven " + test.given + "\n")
			if test.invalid {
				if err == nil {
					t.Fatal("malformed explicit adoption input was accepted")
				}
				return
			}
			if err != nil || len(feature.scenarios) != 1 || len(feature.scenarios[0].steps) != 1 {
				t.Fatalf("explicit source fixture did not parse: %#v %v", feature, err)
			}
			step := feature.scenarios[0].steps[0]
			if step.object != "kr://scene/knowledge" || step.accessInstance != test.instance {
				t.Fatalf("fixture input acquired an implicit selection: %#v", step)
			}
		})
	}
}

func assertSceneRepositoryAccessSelection(t *testing.T, cache *sceneHomeCache, directory, repository, instance string) {
	t.Helper()
	for _, stamp := range sceneRemoteStamps(t, directory) {
		if stamp.id != repository {
			continue
		}
		store, err := snapshot.OpenLakeFS(kernel.RepositoryID(repository), stamp.dsn, cache.lakefs.Credential())
		if err != nil {
			t.Fatal(err)
		}
		if closer, ok := store.(interface{ Close() error }); ok {
			defer closer.Close()
		}
		properties, err := store.(fixture.MetadataSource).ReadRepositoryMetadata()
		if err != nil {
			t.Fatal(err)
		}
		config, err := adoption.ParseRepositoryPlugins(properties)
		if err != nil {
			t.Fatal(err)
		}
		selected := ""
		for _, plugin := range config.Plugins {
			if plugin.Name == "liveaccess" {
				selected = plugin.Instance
			}
		}
		if selected != instance {
			t.Fatalf("native source %s adoption got %q want %q: %#v", repository, selected, instance, properties)
		}
		return
	}
	t.Fatalf("source %s has no frozen native binding", repository)
}

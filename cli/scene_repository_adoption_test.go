package cli_test

import "testing"

// Plugin instances are chosen by deployment, never per repository: the
// existing-repository fixture accepts exactly one id and rejects the retired
// per-repository LiveAccess selection suffix instead of silently ignoring it.
func TestSceneRepositoryFixtureCarriesNoPluginSelection(t *testing.T) {
	for _, test := range []struct {
		given   string
		invalid bool
	}{
		{"existing repository kr://scene/object", false},
		{"existing repository kr://scene/object with LiveAccess instance main", true},
		{"existing repository kr://scene/object extra", true},
		{"existing repository ", true},
	} {
		t.Run(test.given, func(t *testing.T) {
			feature, err := parseSceneFeature("Feature: source\nScenario: fixture\nGiven " + test.given + "\n")
			if test.invalid {
				if err == nil {
					t.Fatal("fixture input carrying a plugin selection or extra words was accepted")
				}
				return
			}
			if err != nil || len(feature.scenarios) != 1 || len(feature.scenarios[0].steps) != 1 {
				t.Fatalf("existing repository fixture did not parse: %#v %v", feature, err)
			}
			if step := feature.scenarios[0].steps[0]; step.kind != "repository-fixture" || step.object != "kr://scene/object" {
				t.Fatalf("fixture input parsed into another step: %#v", step)
			}
		})
	}
}

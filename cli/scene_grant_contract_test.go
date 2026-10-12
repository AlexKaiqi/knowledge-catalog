package cli_test

import (
	"path/filepath"
	"testing"

	"kc/cli"
)

func TestSceneGrantIDUsesSuccessfulReceipt(t *testing.T) {
	world := &sceneWorld{ids: map[string]string{}}
	for _, tc := range []struct {
		name, operation, output, want string
		status                        int
	}{
		{"missing", "grant add", `{}`, "", 0},
		{"receipt", "grant add", `{"id":"opaque-receipt"}`, "opaque-receipt", 0},
		{"failed add", "grant add", `{"id":"failed-receipt"}`, "opaque-receipt", 1},
		{"other command", "catalog list", `{"id":"unrelated-id"}`, "opaque-receipt", 0},
		{"inventory", "grant list", `{"rules":[{"id":"inventory-id"}]}`, "opaque-receipt", 0},
		{"admin receipt", "admin grant add", `{"id":"admin-receipt"}`, "admin-receipt", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world.cli = kcRunResult{RunResult: cli.RunResult{Status: tc.status, Stdout: tc.output}, operation: tc.operation}
			world.captureCLI()
			if got := world.ids["grantId"]; got != tc.want {
				t.Fatalf("captured grantId=%q want %q", got, tc.want)
			}
		})
	}
	if got := world.expandExpected("$grantId"); got != "admin-receipt" {
		t.Fatalf("receipt expectation=%q", got)
	}
	if got, err := world.expandArg("$grantId", ""); err != nil || got != "admin-receipt" {
		t.Fatalf("receipt argument=%q: %v", got, err)
	}
	clone := &sceneWorld{ids: remapSceneIDs(world.ids, "/old-home", "/new-home")}
	if got := clone.expandExpected("$grantId"); got != "admin-receipt" {
		t.Fatalf("frozen receipt expectation=%q", got)
	}
	if err := matchJSONExpect(map[string]any{"id": "different-id"}, "id", clone.expandExpected("$grantId")); err == nil {
		t.Fatal("inventory must not match an ID different from the captured receipt")
	}
	if _, err := (&sceneWorld{ids: map[string]string{}}).expandArg("$grantId", ""); err == nil {
		t.Fatal("uncaptured grantId must not be accepted in argv")
	}
}

func TestSceneCatalogReadGrantKeepsReceiptAndScopeAssertions(t *testing.T) {
	leaf := filepath.Join(scenesRoot(), "service-initialized", "inventory-read-granted")
	for _, tc := range []struct {
		path, command string
		want          map[string]string
	}{
		{filepath.Join(leaf, "_build", "construct.feature"), "kc grant add --principal bot --action inventory.read", map[string]string{
			"id": "nonempty", "principal": "bot", "catalog": "absent", "actions.0": "inventory.read",
		}},
		{filepath.Join(leaf, "_build", "construct.feature"), "kc grant list", map[string]string{
			"rules[].principal": "bot",
			"rules.0.catalog":   "absent", "rules[].actions.0": "inventory.read",
		}},
		{filepath.Join(leaf, "_probes", "probe-not-admin.feature"), "kc grant list", map[string]string{
			"rules.0.id": "$grantId", "rules.0.principal": "bot",
			"rules.0.catalog": "absent", "rules.0.actions.0": "inventory.read",
			"rules.0.repo": "absent", "rules.0.dataset": "absent", "rules.0.ref": "absent",
			"rules.0.object": "absent", "rules.0.aspect": "absent", "rules.1": "absent",
		}},
	} {
		feature, err := parseSceneFeatureFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		command, found := "", map[string]string{}
		for _, scenario := range feature.scenarios {
			for _, step := range scenario.steps {
				if step.kind == "run" {
					command = step.command
				} else if command == tc.command && (step.kind == "output-has" || step.kind == "output-includes") {
					for _, row := range step.table {
						found[row[0]] = row[1]
					}
				}
			}
		}
		for field, want := range tc.want {
			if got := found[field]; got != want {
				t.Errorf("%s lost %s=%q; got %q", tc.path, field, want, got)
			}
		}
	}
}

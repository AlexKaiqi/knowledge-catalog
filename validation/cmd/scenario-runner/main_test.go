package main

import (
	"strings"
	"testing"

	"kc/validation/scenarios"
)

func TestScenarioEnvironmentExpandsSceneRootAndKeepsMySQLBetweenAtoms(t *testing.T) {
	t.Setenv("KC_DW_KEEP_MYSQL", "")
	scenario := scenarios.Scenario{
		ID:  "producer.mysql.structure",
		Env: map[string]string{"REPORT": "${SCENE_ROOT}/.data/report.json"},
	}
	remaining := []scenarios.Scenario{{ID: "producer.mysql.observations"}}
	env := scenarioEnvironment(scenario, remaining, "/scene")
	assertEnv(t, env, "KC_DW_DEPENDENCIES_READY=1")
	assertEnv(t, env, "KC_DW_KEEP_MYSQL=1")
	assertEnv(t, env, "REPORT=/scene/.data/report.json")
}

func TestScenarioEnvironmentCleansMySQLAfterLastAtom(t *testing.T) {
	t.Setenv("KC_DW_KEEP_MYSQL", "")
	env := scenarioEnvironment(scenarios.Scenario{ID: "producer.mysql.cdc"}, nil, "/scene")
	assertEnv(t, env, "KC_DW_KEEP_MYSQL=0")
}

func assertEnv(t *testing.T, env []string, want string) {
	t.Helper()
	for _, item := range env {
		if item == want {
			return
		}
	}
	t.Fatalf("missing %q in environment:\n%s", want, strings.Join(env, "\n"))
}

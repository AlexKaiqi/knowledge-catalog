package cli_test

import (
	"fmt"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectclient "kc/object/client"
)

func sceneHTTPReferenceInputProblems(t *testing.T, scenario sceneScenario) []error {
	t.Helper()
	return sceneHTTPInputProblemsWithValidation(t, scenario, func(path string, payload any) error {
		var request any
		var reference *string
		if path == "/knowledge/v1/objects:read" {
			read := &objectclient.ReadRequest{}
			request, reference = read, &read.Reference
		} else {
			maintenance := &objectclient.MaintenanceRequest{}
			request, reference = maintenance, &maintenance.Reference
		}
		return testkit.ValidateJSONFixture(payload, request, func() error {
			_, err := objectmodel.ParseReference(*reference)
			return err
		})
	})
}

func sceneHTTPInputProblemsWithValidation(t *testing.T, scenario sceneScenario, validate func(string, any) error) []error {
	t.Helper()
	var problems []error
	for i, step := range scenario.steps {
		if step.kind != "http" || step.method != "POST" || sceneHTTPExpectsInvalidInput(scenario.steps[i+1:]) {
			continue
		}
		switch step.path {
		case "/knowledge/v1/objects:read", "/knowledge/v1/objects:put", "/knowledge/v1/objects:remove":
		default:
			continue
		}
		if err := validate(step.path, httpTablePayload(t, step.table)); err != nil {
			problems = append(problems, fmt.Errorf("line %d: %s input does not match its public Object contract: %w", step.line, step.path, err))
		}
	}
	return problems
}

func sceneHTTPExpectsInvalidInput(steps []sceneStep) bool {
	for _, step := range steps {
		switch step.kind {
		case "run", "http", "material", "http-server":
			return false
		case "error":
			return step.errorCode == string(kernel.ErrUsageInvalid)
		}
	}
	return false
}

func TestSceneHTTPReferenceInputGuardKeepsScenarioOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, path, then string
		validate         bool
	}{
		{"authorization needs valid input", "/knowledge/v1/objects:read", "Then error FORBIDDEN", true},
		{"explicit invalid input probe", "/knowledge/v1/objects:read", "Then error USAGE_INVALID", false},
		{"search keeps its own input", "/knowledge/v1/search", "Then 1 hit metric/gmv with full canonical", false},
		{"resolve keeps its own input", "/knowledge/v1/objects:resolve", "Then the command succeeds", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feature, err := parseSceneFeature("Feature: HTTP reference\nScenario: input\nWhen HTTP POST " + tc.path + " as taihu:alice:\n| object | metric/gmv |\n" + tc.then + "\n")
			if err != nil {
				t.Fatal(err)
			}
			called := false
			invalidInput := fmt.Errorf("invalid request sentinel")
			problems := sceneHTTPInputProblemsWithValidation(t, feature.scenarios[0], func(path string, payload any) error {
				called = true
				if path != tc.path || payload.(map[string]any)["object"] != "metric/gmv" {
					t.Fatalf("scenario input changed: %s %#v", path, payload)
				}
				return invalidInput
			})
			if called != tc.validate || (len(problems) > 0) != tc.validate {
				t.Fatalf("validator called=%v, problems=%v, want validation=%v", called, problems, tc.validate)
			}
			if tc.validate && !strings.Contains(problems[0].Error(), invalidInput.Error()) {
				t.Fatalf("scenario must retain its validation failure: %v", problems)
			}
		})
	}
}

package cli_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSceneReadAssertionsUseSelectedObjectShape(t *testing.T) {
	root := scenesRoot()
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == "_results" {
			return fs.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".feature" {
			return nil
		}
		feature, err := parseSceneFeatureFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return nil
		}
		for _, scenario := range feature.scenarios {
			for _, problem := range sceneReadAssertionShapeProblems(scenario) {
				t.Errorf("%s: %v", path, problem)
			}
			for _, problem := range sceneHTTPReferenceInputProblems(t, scenario) {
				t.Errorf("%s: %v", path, problem)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sceneReadAssertionShapeProblems(scenario sceneScenario) []error {
	var problems []error
	reading := false
	for _, step := range scenario.steps {
		switch step.kind {
		case "run":
			args, err := splitSceneArgs(step.command)
			if err != nil {
				problems = append(problems, fmt.Errorf("line %d: %w", step.line, err))
				reading = false
				continue
			}
			reading = args[0] == "read"
		case "http", "http-server", "material":
			reading = false
		case "output-includes":
			if !reading {
				continue
			}
			for _, row := range step.table {
				field := strings.TrimSpace(row[0])
				if strings.HasPrefix(field, "[]") || isSceneReadScalarCoordinate(field) {
					problems = append(problems, fmt.Errorf("line %d: read selects one object; scalar field %q requires output has, not array includes", step.line, field))
				}
			}
		}
	}
	return problems
}

func isSceneReadScalarCoordinate(field string) bool {
	root, _, _ := strings.Cut(field, ".")
	switch strings.TrimSuffix(root, "[]") {
	case "objectId", "repository", "commit", "reference", "locator", "digest", "aspectName", "schemaRef":
		return true
	default:
		return false
	}
}

func TestSceneReadAssertionShapeGuardPreservesArrayContracts(t *testing.T) {
	const read = "When I run `kc read urn:kc:object:note/hello --dataset scene-notes`\n"
	for _, tc := range []struct {
		name, steps string
		reject      bool
	}{
		{"scalar identity", read + "Then the output includes:\n  | objectId | note/hello |\n", true},
		{"scalar source", read + "Then the output includes:\n  | repository | kr://scene/object |\n", true},
		{"scalar version", read + "Then the output includes:\n  | commit | nonempty |\n", true},
		{"former root array", read + "Then the output includes:\n  | [].objectId | note/hello |\n", true},
		{"selected object equality", read + "Then the output has:\n  | objectId | note/hello |\n  | commit | nonempty |\n", false},
		{"business array", read + "Then the output includes:\n  | value.tags | tea |\n", false},
		{"business array members", read + "Then the output includes:\n  | value.columns[].name | order_id |\n", false},
		{"unit declaration array", read + "Then the output includes:\n  | declarations[].schemaRef | schema/note |\n", false},
		{"search array", "When I run `kc search --dataset scene-notes --query tea`\nThen the output includes:\n  | hits[].objectId | note/hello |\n", false},
		{"resolve array", "When I run `kc resolve --dataset scene-notes --object note/hello`\nThen the output includes:\n  | [].status | RESOLVED |\n", false},
		{"next command owns output", read + "Then the output has:\n  | objectId | note/hello |\nWhen I run `kc resolve --dataset scene-notes --object note/hello`\nThen the output includes:\n  | [].status | RESOLVED |\n", false},
		{"HTTP owns next output", read + "Then the output has:\n  | objectId | note/hello |\nWhen HTTP POST /object/v1/search\nThen the output includes:\n  | hits[].objectId | note/hello |\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feature, err := parseSceneFeature("Feature: read shape\nScenario: shape\n" + tc.steps)
			if err != nil {
				t.Fatal(err)
			}
			problems := sceneReadAssertionShapeProblems(feature.scenarios[0])
			if (len(problems) > 0) != tc.reject {
				t.Fatalf("reject=%v, want %v: %v", len(problems) > 0, tc.reject, problems)
			}
		})
	}
}

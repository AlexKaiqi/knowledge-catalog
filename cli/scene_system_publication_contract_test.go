package cli_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kc/knowledge"
)

// Publication changes must update every scene's system inventory expectation,
// while retaining exact counts for both system and ordinary repositories.
func TestSceneSystemInventoryMatchesPublishedSchemas(t *testing.T) {
	want := strconv.Itoa(len(knowledge.SystemSchemaOperations()))
	checked := map[string]int{}
	err := filepath.WalkDir(scenesRoot(), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".feature") {
			return err
		}
		feature, err := parseSceneFeatureFile(path)
		if err != nil {
			return err
		}
		for _, scenario := range feature.scenarios {
			checked[path] += assertSceneSystemInventory(t, path, scenario, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"service-initialized/_build/construct.feature",
		"service-initialized/source-repositories-configured/repository-attached/_build/construct.feature",
	} {
		if checked[filepath.Join(scenesRoot(), filepath.FromSlash(required))] == 0 {
			t.Errorf("%s must retain an exact system identity and Schema count assertion", required)
		}
	}
}

func assertSceneSystemInventory(t *testing.T, path string, scenario sceneScenario, want string) int {
	t.Helper()
	checked := 0
	rows := map[string]string{}
	check := func() {
		for field, value := range rows {
			if !strings.HasSuffix(field, ".id") || value != string(knowledge.SystemRepositoryID) {
				continue
			}
			count := strings.TrimSuffix(field, ".id") + ".schemaCount"
			got, exists := rows[count]
			if !exists {
				continue
			}
			checked++
			if got != want {
				t.Errorf("%s scenario %q: %s is %s, published system Schemas require %s", path, scenario.name, count, got, want)
			}
		}
		rows = map[string]string{}
	}
	for _, step := range scenario.steps {
		if step.kind == "run" || step.kind == "http" {
			check()
		}
		if step.kind == "output-has" || step.kind == "output-includes" {
			for _, row := range step.table {
				if len(row) == 2 {
					rows[row[0]] = row[1]
				}
			}
		}
	}
	check()
	return checked
}

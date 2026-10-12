package home

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDeploymentRejectsCatalogCollection(t *testing.T) {
	cfg := deploymentFixture(t)
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := yaml.Unmarshal(encoded, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["catalogs"] = []any{legacy["registry"]}
	delete(legacy, "registry")
	raw, err := yaml.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deployment.yaml")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDeployment(path); err == nil {
		t.Fatal("deployment accepted a Catalog collection instead of one registry")
	}
}

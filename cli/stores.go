package cli

import (
	"os"
	"path/filepath"
	"strings"

	"kc/retrieval/opensearch"
)

const (
	defaultRepositoryDriver = "dolt"
	defaultIndexDriver      = "none"
	defaultReposDir         = "repos"
	defaultCatalogsDir      = "catalogs"
	defaultProjectionsDir   = "projections"
	defaultCheckoutsDir     = "checkouts"
	legacyCatalogDir        = "repos/_catalog"
	legacyCatalogsDir       = "repos/_catalogs"
)

// LayoutFile is .kc/layout.yaml: this machine's directories.
type LayoutFile struct {
	Repos       string `json:"repos,omitempty" yaml:"repos"`
	Catalog     string `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Catalogs    string `json:"catalogs,omitempty" yaml:"catalogs"`
	Projections string `json:"projections,omitempty" yaml:"projections"`
	Checkouts   string `json:"checkouts,omitempty" yaml:"checkouts"`
}

// StoresFile is the merged runtime representation of layout.yaml and stores.yaml.
type StoresFile struct {
	Layout     LayoutFile        `json:"layout,omitempty" yaml:"layout,omitempty"`
	Profile    string            `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository string            `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index      string            `json:"index,omitempty" yaml:"index,omitempty"`
	OpenSearch opensearch.Config `json:"opensearch,omitempty" yaml:"opensearch,omitempty"`
}

func storesPath(home string) string {
	return filepath.Join(home, "stores.yaml")
}

func layoutPath(home string) string {
	return filepath.Join(home, "layout.yaml")
}

func legacyStoresJSONPath(home string) string {
	return filepath.Join(home, "stores.json")
}

func DefaultLayout() LayoutFile {
	return LayoutFile{Repos: defaultReposDir, Catalogs: defaultCatalogsDir, Projections: defaultProjectionsDir, Checkouts: defaultCheckoutsDir}
}

// DefaultStores returns local Dolt without a retrieval projection.
// Service deployments select OpenSearch explicitly.
func DefaultStores() StoresFile {
	stores := StoresFile{
		Layout: DefaultLayout(), Profile: "local",
		Repository: defaultRepositoryDriver, Index: defaultIndexDriver,
	}
	// The repository test suite exercises the deployed provider rather than a
	// second local implementation. testsuite.sh owns this explicit endpoint.
	if endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")); endpoint != "" {
		stores.Index = "opensearch"
		stores.OpenSearch.URL = endpoint
	}
	return stores.withDefaults()
}

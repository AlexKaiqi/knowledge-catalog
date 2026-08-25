package cli

import (
	"path/filepath"

	"kc/retrieval/elasticsearch"
	"kc/retrieval/starrocks"
)

const (
	defaultRepositoryDriver = "filegit"
	defaultIndexDriver      = "sqlite"
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

// StoresFile is the merged in-memory view of layout.yaml and stores.yaml.
type StoresFile struct {
	Layout        LayoutFile           `json:"layout,omitempty" yaml:"layout,omitempty"`
	Profile       string               `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository    string               `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index         string               `json:"index,omitempty" yaml:"index,omitempty"`
	Elasticsearch elasticsearch.Config `json:"elasticsearch,omitempty" yaml:"elasticsearch,omitempty"`
	StarRocks     starrocks.Config     `json:"starrocks,omitempty" yaml:"starrocks,omitempty"`
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

// DefaultStores returns local FileGit + SQLite engines and the default layout.
func DefaultStores() StoresFile {
	return StoresFile{
		Layout: DefaultLayout(), Profile: "local",
		Repository: defaultRepositoryDriver, Index: defaultIndexDriver,
	}.withDefaults()
}

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/declarativeindex/opensearch"
	indexplugins "kc/discovery/plugin"
)

const (
	// DefaultRepositoryDriver is the only Snapshot authority driver shipped.
	DefaultRepositoryDriver = DriverLakeFS
	// DefaultIndexDriver selects no retrieval projection.
	DefaultIndexDriver    = "none"
	DefaultReposDir       = "repos"
	DefaultRegistryDir    = "registry"
	DefaultProjectionsDir = "projections"
	// DefaultCheckoutsDir is where a mounted consumption view's checkouts live.
	DefaultCheckoutsDir = "checkouts"

	legacyStoresFile = "stores.json"
)

// LayoutFile is .kc/layout.yaml: this machine's directories.
type LayoutFile struct {
	Repos       string `json:"repos,omitempty" yaml:"repos"`
	Registry    string `json:"registry,omitempty" yaml:"registry"`
	Projections string `json:"projections,omitempty" yaml:"projections"`
	Checkouts   string `json:"checkouts,omitempty" yaml:"checkouts"`
}

// StoresFile is the merged runtime representation of layout.yaml and stores.yaml.
type StoresFile struct {
	Layout         LayoutFile            `json:"layout,omitempty" yaml:"layout,omitempty"`
	Profile        string                `json:"profile,omitempty" yaml:"profile,omitempty"`
	Repository     string                `json:"repository,omitempty" yaml:"repository,omitempty"`
	Index          string                `json:"index,omitempty" yaml:"index,omitempty"`
	OpenSearch     opensearch.Config     `json:"opensearch,omitempty" yaml:"opensearch,omitempty"`
	HydrationCache *HydrationCacheConfig `json:"hydrationCache,omitempty" yaml:"hydrationCache,omitempty"`
	// Plugins is the installed plugin instances. Nil installs the compiled
	// defaults; an explicit empty map installs none.
	Plugins *indexplugins.PluginsConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

// HydrationCacheConfig controls process-local Snapshot body acceleration.
// An omitted configuration enables bounded caching and hot-object warmup.
// Cold-start identity paging is enabled explicitly with ColdStart.
type HydrationCacheConfig struct {
	Disabled      bool  `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	MaxBytes      int64 `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
	MaxEntries    int   `json:"maxEntries,omitempty" yaml:"maxEntries,omitempty"`
	WarmLimit     int   `json:"warmLimit,omitempty" yaml:"warmLimit,omitempty"`
	WarmBatchSize int   `json:"warmBatchSize,omitempty" yaml:"warmBatchSize,omitempty"`
	ColdStart     *bool `json:"coldStart,omitempty" yaml:"coldStart,omitempty"`
}

// DefaultLayout is the local profile's directory set.
func DefaultLayout() LayoutFile {
	return LayoutFile{Repos: DefaultReposDir, Registry: DefaultRegistryDir, Projections: DefaultProjectionsDir, Checkouts: DefaultCheckoutsDir}
}

// DefaultStores returns the local profile with the lakeFS repository driver
// and no retrieval projection. Service deployments select OpenSearch
// explicitly. Repository drivers own no local authority directory; repo-add
// supplies the existing authority's --dsn.
func DefaultStores() StoresFile {
	stores := StoresFile{
		Layout: DefaultLayout(), Profile: "local",
		Repository: DefaultRepositoryDriver, Index: DefaultIndexDriver,
	}
	// The repository test suite exercises the deployed provider rather than a
	// second local implementation. testsuite.sh owns this explicit endpoint.
	if endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")); endpoint != "" {
		stores.Index = "opensearch"
		stores.OpenSearch.URL = endpoint
		// The suite creates many disposable repositories on one local node.
		// Production scale defaults remain in opensearch.Config.WithDefaults.
		stores.OpenSearch.PrimaryShards = 1
	}
	return stores.WithDefaults()
}

// WithDefaults fills every unset store and layout field with its documented
// default. It never invents a provider: an unknown driver stays as written so
// ValidateProfile can refuse it.
func (s StoresFile) WithDefaults() StoresFile {
	if s.Profile == "" {
		s.Profile = "local"
	}
	if s.Repository == "" {
		s.Repository = DefaultRepositoryDriver
	}
	s.Repository = NormalizeRepositoryDriver(s.Repository)
	if s.Index == "" {
		if s.Profile == "scale" {
			s.Index = "opensearch"
		} else {
			s.Index = DefaultIndexDriver
		}
	}
	s.Index = NormalizeIndexDriver(s.Index)
	if s.Layout.Repos == "" {
		s.Layout.Repos = DefaultReposDir
	}
	if s.Layout.Registry == "" {
		s.Layout.Registry = DefaultRegistryDir
	}
	if s.Layout.Projections == "" {
		s.Layout.Projections = DefaultProjectionsDir
	}
	if s.Layout.Checkouts == "" {
		s.Layout.Checkouts = DefaultCheckoutsDir
	}
	return s
}

// ValidateProfile refuses a store selection this build cannot honour: it is
// static only, so it reads the driver names rather than opening an authority.
func (s StoresFile) ValidateProfile() error {
	if c := s.HydrationCache; c != nil && (c.MaxBytes < 0 || c.MaxEntries < 0 || c.WarmLimit < 0 || c.WarmBatchSize < 0) {
		return fmt.Errorf("hydration cache limits must not be negative")
	}
	switch s.Profile {
	case "", "local", "scale":
	default:
		return fmt.Errorf("unknown store profile %s (want local or scale)", s.Profile)
	}
	if s.Repository == "postgres" {
		return ErrUnsupportedDriver("repository", s.Repository)
	}
	if err := RejectNonRepository(s.Repository); err != nil {
		return err
	}
	if _, err := LookupAuthority(s.Repository); err != nil {
		return err
	}
	switch s.Index {
	case "none", "opensearch":
	default:
		return fmt.Errorf("%s is not a Knowledge Catalog projection provider", s.Index)
	}
	return nil
}

// RejectSecrets refuses connection secrets in a file-backed store selection.
func (s StoresFile) RejectSecrets() error {
	return s.OpenSearch.RejectSecrets()
}

// NormalizeRepositoryDriver folds the spellings operators already use.
func NormalizeRepositoryDriver(raw string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(raw)); normalized {
	case "", "local":
		return DefaultRepositoryDriver
	case "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return normalized
	}
}

// NormalizeIndexDriver folds the spellings operators already use.
func NormalizeIndexDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return "none"
	case "opensearch":
		return "opensearch"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// NormalizeProfile accepts only the profiles this build defines.
func NormalizeProfile(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "local":
		return "local", nil
	case "scale":
		return "scale", nil
	default:
		return "", fmt.Errorf("unknown store profile %s (want local or scale)", raw)
	}
}

// ErrUnsupportedDriver turns the known category errors into their own message.
func ErrUnsupportedDriver(kind, driver string) error {
	if kind == "store" {
		return fmt.Errorf("unknown store driver %s: use a configured repository or projection driver", driver)
	}
	return fmt.Errorf("unknown %s driver %s: snapshot repositories support %s", kind, driver, DriverLakeFS)
}

// RejectNonRepository reports a driver from another layer as its own category
// error instead of a typo, so the refusal keeps its meaning.
func RejectNonRepository(driver string) error {
	switch driver {
	case "mysql":
		return ErrUnsupportedDriver("repository", driver)
	case "postgres":
		return ErrUnsupportedDriver("repository", driver)
	}
	return nil
}

// StoresPath is where a home keeps its store selection.
func StoresPath(home string) string { return filepath.Join(home, "stores.yaml") }

// LayoutPath is where a home keeps its directory set.
func LayoutPath(home string) string { return filepath.Join(home, "layout.yaml") }

// LegacyStoresPath is the retired JSON location a write removes and a read
// still honours so an existing home keeps opening.
func LegacyStoresPath(home string) string { return filepath.Join(home, legacyStoresFile) }

// ResolveStoreDir joins a layout directory with home unless it is absolute.
func ResolveStoreDir(home, dir, fallback string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		dir = fallback
	}
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	clean := filepath.Clean(dir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("store dir %q must stay under --home", dir)
	}
	return filepath.Join(home, clean), nil
}

// MergeLayout fills unset layout fields from a fallback, keeping explicit ones.
func MergeLayout(primary, fallback LayoutFile) LayoutFile {
	if primary.Repos == "" {
		primary.Repos = fallback.Repos
	}
	if primary.Registry == "" {
		primary.Registry = fallback.Registry
	}
	if primary.Projections == "" {
		primary.Projections = fallback.Projections
	}
	if primary.Checkouts == "" {
		primary.Checkouts = fallback.Checkouts
	}
	return primary
}

package home

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"

	"gopkg.in/yaml.v3"
)

// DeploymentConfig is an immutable deployment input. StateDir is an explicit
// durable volume; CacheDir is disposable instance storage. Neither is an
// inventory: Catalog membership is read only from the configured Git authority.
type DeploymentConfig struct {
	Version             int                                `json:"version" yaml:"version"`
	StateDir            string                             `json:"stateDir" yaml:"stateDir"`
	CacheDir            string                             `json:"cacheDir" yaml:"cacheDir"`
	Catalogs            []CatalogBinding                   `json:"catalogs" yaml:"catalogs"`
	Repositories        []RepositoryBinding                `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	ManagedRepositories *ManagedRepositoryConfig           `json:"managedRepositories,omitempty" yaml:"managedRepositories,omitempty"`
	ManagedStores       map[string]ManagedRepositoryConfig `json:"managedStores,omitempty" yaml:"managedStores,omitempty"`
	Admission           *AdmissionConfig                   `json:"admission,omitempty" yaml:"admission,omitempty"`
	Connections         *ConnectionPolicy                  `json:"connections,omitempty" yaml:"connections,omitempty"`
	BootstrapPrincipal  string                             `json:"bootstrapPrincipal,omitempty" yaml:"bootstrapPrincipal,omitempty"`
	Stores              StoresFile                         `json:"stores,omitempty" yaml:"stores,omitempty"`
	Auth                string                             `json:"auth" yaml:"auth"`
	AuthURL             string                             `json:"authURL,omitempty" yaml:"authURL,omitempty"`
	Listen              string                             `json:"listen,omitempty" yaml:"listen,omitempty"`
}

// CatalogBinding locates an existing Git repository. Initialization may create
// its Catalog ref explicitly; opening a deployment never creates that ref.
type CatalogBinding struct {
	ID                   string `json:"id" yaml:"id"`
	Remote               string `json:"remote" yaml:"remote"`
	Ref                  string `json:"ref,omitempty" yaml:"ref,omitempty"`
	DiscoveryWorkspaceID string `json:"discoveryWorkspaceId,omitempty" yaml:"discoveryWorkspaceId,omitempty"`
}

// RepositoryBinding is an operator-approved source, not Catalog membership.
// Credentials are supplied by the adapter's deployment credential provider.
type RepositoryBinding struct {
	ID     string `json:"id" yaml:"id"`
	Driver string `json:"driver" yaml:"driver"`
	DSN    string `json:"dsn,omitempty" yaml:"dsn,omitempty"`
	Dir    string `json:"dir,omitempty" yaml:"dir,omitempty"`
}

func ReadDeployment(path string) (DeploymentConfig, error) {
	if strings.TrimSpace(path) == "" {
		return DeploymentConfig{}, kernel.Fail(kernel.ErrUsageInvalid, "deployment requires --config")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return DeploymentConfig{}, err
	}
	var config DeploymentConfig
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return DeploymentConfig{}, kernel.Fail(kernel.ErrUsageInvalid, "deployment configuration: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return DeploymentConfig{}, kernel.Fail(kernel.ErrUsageInvalid, "deployment configuration must contain one YAML document")
	}
	if err := config.Validate(); err != nil {
		return DeploymentConfig{}, err
	}
	return config, nil
}

func (c DeploymentConfig) Validate() error {
	invalid := func(format string, args ...any) error { return kernel.Fail(kernel.ErrUsageInvalid, format, args...) }
	if c.Version != 1 {
		return invalid("deployment version must be 1")
	}
	if !filepath.IsAbs(c.StateDir) || !filepath.IsAbs(c.CacheDir) {
		return invalid("stateDir and cacheDir must be explicit absolute paths")
	}
	if pathsOverlap(c.StateDir, c.CacheDir) {
		return invalid("durable stateDir and disposable cacheDir must be separate, non-nested paths")
	}
	if len(c.Catalogs) == 0 {
		return invalid("deployment must name at least one Catalog Git authority")
	}
	if c.Auth != "local" && c.Auth != "taihu" && c.Auth != "gitea" {
		return invalid("deployment auth must be local, taihu, or gitea")
	}
	if err := validateManagedConfig(c); err != nil {
		return err
	}
	if err := validateAdmissionConfig(c); err != nil {
		return err
	}
	if err := validateConnectionPolicy(c.Connections); err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, b := range c.Catalogs {
		if b.DiscoveryWorkspaceID != strings.TrimSpace(b.DiscoveryWorkspaceID) || strings.ContainsAny(b.DiscoveryWorkspaceID, "\r\n") {
			return invalid("discoveryWorkspaceId must be an exact Workspace identity")
		}
		id, err := NormalizeCatalogID(b.ID)
		if err != nil || id != b.ID || ids[id] || id == string(knowledge.SystemRepositoryID) {
			return invalid("Catalog binding has invalid or duplicate identity %q", b.ID)
		}
		ids[id] = true
		if strings.TrimSpace(b.Remote) == "" || strings.HasPrefix(b.Remote, "-") {
			return invalid("Catalog %s requires a Git remote", id)
		}
		if err := snapshot.RejectConfiguredSecret("git", b.Remote, "Git credential helper"); err != nil {
			return err
		}
		location := b.Remote
		parsed, parseErr := url.Parse(b.Remote)
		if parseErr != nil {
			return invalid("invalid Catalog Git remote")
		}
		if parsed.Scheme == "file" {
			location = parsed.Path
		}
		isPath := parsed.Scheme == "file" || !strings.Contains(b.Remote, ":")
		if isPath && !filepath.IsAbs(location) {
			return invalid("Catalog filesystem authority requires an absolute path")
		}
		if isPath && (pathsOverlap(location, c.CacheDir) || pathsOverlap(location, c.StateDir)) {
			return invalid("Catalog Git authority must be independent of instance cache and control state")
		}
		if b.Ref != "" && !strings.HasPrefix(b.Ref, "refs/heads/") {
			return invalid("Catalog ref must be a full branch ref")
		}
	}
	for _, b := range c.Repositories {
		id, err := NormalizeCatalogID(b.ID)
		if err != nil || id != b.ID || ids[id] {
			return invalid("Snapshot binding has invalid, reserved, or duplicate identity %q", b.ID)
		}
		ids[id] = true
		if b.Driver != "dolt" && b.Driver != "gitea" {
			return invalid("Snapshot binding requires explicit driver dolt or gitea")
		}
		if b.Driver == "dolt" && (b.Dir == "" || b.DSN != "") {
			return invalid("Dolt Snapshot binding requires dir and does not accept dsn")
		}
		if b.Driver == "gitea" && b.Dir != "" {
			return invalid("Gitea Snapshot binding requires dsn and does not accept dir")
		}
		driver, err := authorityFor(b.Driver)
		if err != nil {
			return err
		}
		item := b.homeRepo()
		if driver.validate != nil {
			if err := driver.validate(item); err != nil {
				return err
			}
		}
		if b.Dir != "" && (!filepath.IsAbs(b.Dir) || pathsOverlap(b.Dir, c.CacheDir)) {
			return invalid("Snapshot directory must be absolute and independent of the instance cache")
		}
		if b.Dir == "" && b.DSN == "" {
			return invalid("Snapshot %s requires an existing authority location", b.ID)
		}
	}
	if c.Stores.Layout != (LayoutFile{}) || c.Stores.Profile != "" || c.Stores.Repository != "" {
		return invalid("deployment stores accepts index, opensearch and hydrationCache; configure source drivers in repositories and paths in stateDir/cacheDir")
	}
	if err := c.runtimeStores().ValidateProfile(); err != nil {
		return err
	}
	if err := c.Stores.rejectSecrets(); err != nil {
		return err
	}
	return nil
}

func (b RepositoryBinding) homeRepo() HomeRepo {
	return HomeRepo{ID: b.ID, Driver: b.Driver, DSN: b.DSN, Dir: b.Dir}
}

func (c DeploymentConfig) runtimeStores() StoresFile {
	stores := c.Stores.withDefaults()
	stores.Layout.Catalogs = filepath.Join(c.CacheDir, "catalogs")
	stores.Layout.Projections = filepath.Join(c.CacheDir, "projections")
	stores.Layout.Checkouts = filepath.Join(c.CacheDir, "checkouts")
	return stores
}

func pathsOverlap(a, b string) bool {
	a, b = deploymentRealPath(a), deploymentRealPath(b)
	inside := func(root, p string) bool {
		rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return inside(a, b) || inside(b, a)
}

func (c DeploymentConfig) CatalogCache(binding CatalogBinding) string {
	// Digest avoids the collisions inherent in the legacy directory scanner.
	return filepath.Join(c.CacheDir, "catalogs", string(kernel.CanonicalDigest(binding.ID)))
}

func (c DeploymentConfig) Binding(id kernel.RepositoryID) (RepositoryBinding, error) {
	for _, binding := range c.Repositories {
		if binding.ID == string(id) {
			return binding, nil
		}
	}
	return RepositoryBinding{}, kernel.Fail(kernel.ErrPreconditionFailed, "repository %s has no configured Snapshot binding", id)
}

func (c DeploymentConfig) String() string {
	return fmt.Sprintf("deployment with %d Catalogs and %d Snapshot bindings", len(c.Catalogs), len(c.Repositories))
}

func deploymentRealPath(p string) string {
	p = filepath.Clean(p)
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(deploymentRealPath(parent), filepath.Base(p))
}

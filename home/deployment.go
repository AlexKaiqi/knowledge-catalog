package home

import (
	"bytes"
	"fmt"
	"io"
	indexplugins "kc/discovery/plugin"
	"os"
	"path/filepath"
	"strings"

	"kc/identity"
	_ "kc/identity/builtin"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/observability"

	"gopkg.in/yaml.v3"
)

// DeploymentConfig is an immutable deployment input. StateDir is an explicit
// durable volume; CacheDir is disposable instance storage. Neither is an
// inventory: Registry membership is read only from the configured Snapshot
// authority.
type DeploymentConfig struct {
	Version             int                                `json:"version" yaml:"version"`
	StateDir            string                             `json:"stateDir" yaml:"stateDir"`
	CacheDir            string                             `json:"cacheDir" yaml:"cacheDir"`
	Registry            RegistryBinding                    `json:"registry" yaml:"registry"`
	Repositories        []RepositoryBinding                `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	RepositoryAccess    []RepositoryAccess                 `json:"repositoryAccess,omitempty" yaml:"repositoryAccess,omitempty"`
	DatasetAccess       []DatasetAccess                    `json:"datasetAccess,omitempty" yaml:"datasetAccess,omitempty"`
	ManagedRepositories *ManagedRepositoryConfig           `json:"managedRepositories,omitempty" yaml:"managedRepositories,omitempty"`
	ManagedStores       map[string]ManagedRepositoryConfig `json:"managedStores,omitempty" yaml:"managedStores,omitempty"`
	Admission           *AdmissionConfig                   `json:"admission,omitempty" yaml:"admission,omitempty"`
	Connections         *ConnectionPolicy                  `json:"connections,omitempty" yaml:"connections,omitempty"`
	BootstrapPrincipal  string                             `json:"bootstrapPrincipal,omitempty" yaml:"bootstrapPrincipal,omitempty"`
	Stores              StoresFile                         `json:"stores,omitempty" yaml:"stores,omitempty"`
	Evidence            *EvidenceConfig                    `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	LiveAccess          liveaccess.RuntimeConfig           `json:"liveaccess,omitempty" yaml:"liveaccess,omitempty"`
	// Plugins installs plugin instances and maps each catalog to one of them.
	Plugins       *indexplugins.PluginsConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
	Auth          string                      `json:"auth" yaml:"auth"`
	AuthURL       string                      `json:"authURL,omitempty" yaml:"authURL,omitempty"`
	AuthLogoutURL string                      `json:"authLogoutURL,omitempty" yaml:"authLogoutURL,omitempty"`
	Listen        string                      `json:"listen,omitempty" yaml:"listen,omitempty"`
}

// EvidenceConfig is the ILM-style hot window for access evidence on this
// volume. Empty values use observability.FileStore defaults (30d / 1GiB).
type EvidenceConfig struct {
	HotRetention string `json:"hotRetention,omitempty" yaml:"hotRetention,omitempty"`
	MaxBytes     int64  `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
}

// RegistryBinding locates an independent Snapshot authority that stores Registry
// registry files. It is not an Object Repository and must not reuse a member
// repository identity. Initialization may create the Registry ref; opening a
// deployment never creates that ref.
type RegistryBinding struct {
	ID             string `json:"id" yaml:"id"`
	Driver         string `json:"driver" yaml:"driver"`
	DSN            string `json:"dsn,omitempty" yaml:"dsn,omitempty"`
	Dir            string `json:"dir,omitempty" yaml:"dir,omitempty"`
	Ref            string `json:"ref,omitempty" yaml:"ref,omitempty"`
	DiscoverySetID string `json:"discoveryWorkspaceId,omitempty" yaml:"discoveryWorkspaceId,omitempty"`
}

// RepositoryBinding is an operator-approved source, not Registry membership.
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
	if err := rejectRepositoryPluginSelection(raw); err != nil {
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
	if c.Registry.ID == "" {
		return invalid("deployment must name one registry Snapshot authority")
	}
	if _, ok := identity.Lookup(c.Auth); !ok {
		return invalid("deployment auth must be one of %s", strings.Join(identity.Names(), ", "))
	}
	if err := validateAuthLogoutURL(c.AuthLogoutURL); err != nil {
		return err
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
	if err := validateRepositoryAccess(c.RepositoryAccess); err != nil {
		return err
	}
	if err := validateDatasetAccess(c.DatasetAccess); err != nil {
		return err
	}
	if err := c.validateSnapshotBindings(); err != nil {
		return err
	}
	if c.Stores.Plugins != nil {
		return invalid("deployment installs plugins at top-level plugins, not under stores")
	}
	if c.Plugins != nil {
		if err := c.Plugins.Validate([]string{"declarative-index", "liveaccess"}); err != nil {
			return err
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
	if err := c.validateEvidence(); err != nil {
		return err
	}
	if err := c.LiveAccess.Validate(); err != nil {
		return err
	}
	if _, err := c.Binding(objectmodel.SystemRepositoryID); err != nil {
		return invalid("deployment requires an explicit Snapshot binding for %s", objectmodel.SystemRepositoryID)
	}
	return nil
}

func (c DeploymentConfig) validateSnapshotBindings() error {
	invalid := func(format string, args ...any) error { return kernel.Fail(kernel.ErrUsageInvalid, format, args...) }
	ids := map[string]bool{}
	{
		b := c.Registry
		if b.DiscoverySetID != strings.TrimSpace(b.DiscoverySetID) || strings.ContainsAny(b.DiscoverySetID, "\r\n") {
			return invalid("discoveryWorkspaceId must be an exact Workspace identity")
		}
		id, err := NormalizeRepositoryID(b.ID)
		if err != nil || id != b.ID || ids[id] || id == string(objectmodel.SystemRepositoryID) {
			return invalid("registry binding has invalid or duplicate identity %q", b.ID)
		}
		ids[id] = true
		if b.Driver != "lakefs" {
			return invalid("registry %s requires Snapshot driver lakefs", id)
		}
		if b.Dir != "" {
			return invalid("%s registry authority requires dsn and does not accept dir", b.Driver)
		}
		if b.DSN == "" {
			return invalid("registry %s requires an existing Snapshot authority location", id)
		}
		item := b.homeRepo()
		driver, err := authorityFor(b.Driver)
		if err != nil {
			return err
		}
		if driver.validate != nil {
			if err := driver.validate(item); err != nil {
				return err
			}
		}
		if b.Ref != "" && !strings.HasPrefix(b.Ref, "refs/heads/") {
			return invalid("registry ref must be a full branch ref")
		}
	}
	for _, b := range c.Repositories {
		id, err := NormalizeRepositoryID(b.ID)
		if err != nil || id != b.ID || ids[id] {
			return invalid("Snapshot binding has invalid, reserved, or duplicate identity %q", b.ID)
		}
		ids[id] = true
		if b.Driver != "lakefs" {
			return invalid("Snapshot binding requires explicit driver lakefs")
		}
		if b.Dir != "" {
			return invalid("%s Snapshot binding requires dsn and does not accept dir", b.Driver)
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
	return nil
}

func (c DeploymentConfig) validateEvidence() error {
	if c.Evidence == nil {
		return nil
	}
	return observability.ValidateFileStorePolicy(observability.FileStorePolicy{
		HotRetention: c.Evidence.HotRetention,
		MaxBytes:     c.Evidence.MaxBytes,
	})
}

func (b RegistryBinding) homeRepo() HomeRepo {
	return HomeRepo{ID: b.ID, Driver: b.Driver, DSN: b.DSN, Dir: b.Dir}
}

func (b RepositoryBinding) homeRepo() HomeRepo {
	return HomeRepo{ID: b.ID, Driver: b.Driver, DSN: b.DSN, Dir: b.Dir}
}

func (c DeploymentConfig) runtimeStores() StoresFile {
	stores := c.Stores.withDefaults()
	stores.Plugins = c.Plugins
	stores.Layout.Registry = filepath.Join(c.CacheDir, "registry")
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

func (c DeploymentConfig) RegistryCache(binding RegistryBinding) string {
	// Digest avoids the collisions inherent in the legacy directory scanner.
	return filepath.Join(c.CacheDir, "registry", string(kernel.CanonicalDigest(binding.ID)))
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
	return fmt.Sprintf("deployment with one registry and %d Snapshot bindings", len(c.Repositories))
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

// rejectRepositoryPluginSelection refuses the retired per-Repository plugin
// choice instead of letting strict decoding report an unrelated shape error.
func rejectRepositoryPluginSelection(raw []byte) error {
	var document struct {
		Repositories yaml.Node `yaml:"repositories"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil
	}
	var nodes []*yaml.Node
	switch document.Repositories.Kind {
	case yaml.SequenceNode:
		nodes = document.Repositories.Content
	case yaml.MappingNode:
		for i := 1; i < len(document.Repositories.Content); i += 2 {
			nodes = append(nodes, document.Repositories.Content[i])
		}
	}
	for _, node := range nodes {
		if node.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "plugins" {
				return kernel.Fail(kernel.ErrUsageInvalid, "per-Repository plugin selection is retired: install instances in top-level plugins and map each catalog to one instance")
			}
		}
	}
	return nil
}

package config

import (
	"net/url"
	"slices"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

// ManagedRepositoryConfig delegates allocation in one deployment-owned pool.
// It is not a Repository binding; individual allocations live in durable state.
type ManagedRepositoryConfig struct {
	Driver         string   `json:"driver" yaml:"driver"`
	Root           string   `json:"root,omitempty" yaml:"root,omitempty"`
	DSN            string   `json:"dsn,omitempty" yaml:"dsn,omitempty"`
	PublicURL      string   `json:"publicURL,omitempty" yaml:"publicURL,omitempty"`
	CreatorActions []string `json:"creatorActions" yaml:"creatorActions"`
	ShareActions   []string `json:"shareActions,omitempty" yaml:"shareActions,omitempty"`
}

// AdmissionConfig points users at the deployment's external access workflow.
// KC reports current grants but does not host an application or approval queue.
type AdmissionConfig struct {
	RequestURL string `json:"requestURL,omitempty" yaml:"requestURL,omitempty"`
}

// ValidateAdmissionConfig refuses an admission pointer without its URL.
func ValidateAdmissionConfig(p *AdmissionConfig) error {
	if p == nil {
		return nil
	}
	if p.RequestURL == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "admission: requestURL is required")
	}
	return nil
}

// ValidateManagedConfig refuses a provisioning declaration this build cannot
// honour. It is static: it reads driver names rather than opening a pool.
func ValidateManagedConfig(c DeploymentConfig) error {
	return c.validateManagedConfig()
}

func (c DeploymentConfig) validateManagedConfig() error {
	if c.ManagedRepositories != nil && len(c.ManagedStores) != 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "configure managedRepositories or managedStores, not both")
	}
	for name, pool := range c.ManagedStores {
		if !validManagedUsername(name) {
			return kernel.Fail(kernel.ErrUsageInvalid, "managed store name must be a stable nonempty key")
		}
		if err := c.validateManagedPool(pool); err != nil {
			return err
		}
	}
	pool := c.ManagedRepositories
	if pool == nil {
		return nil
	}
	return c.validateManagedPool(*pool)
}

func (c DeploymentConfig) validateManagedPool(pool ManagedRepositoryConfig) error {
	if err := ValidateCreatorActions(pool.CreatorActions); err != nil {
		return err
	}
	if err := ValidateShareActions(pool.ShareActions); err != nil {
		return err
	}
	if pool.Driver == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed repositories require an explicit supported driver")
	}
	if pool.PublicURL != "" && !validManagedPublicURL(pool.PublicURL) {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed publicURL must be an http(s) URL without credentials, query or fragment")
	}
	return ValidateManagedPool(pool)
}

// ValidateCreatorActions refuses an unknown, duplicated, or empty policy.
func ValidateCreatorActions(actions []string) error {
	allowed := []string{"writer.preview", "writer.commit", "writer.receipt.read", "repository.metadata.read", "repository.shares.manage", "repositories.manage", "writer.branch", "writer.merge", "projection.read", "dataset.resolve", "file.read", "object.read", "object.provenance", "object.history.read", "object.schema.read", "object.relations"}
	if len(actions) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "managed repositories require an explicit nonempty creatorActions policy")
	}
	seen := map[string]bool{}
	for _, action := range actions {
		if (!slices.Contains(allowed, action) && !access.IsRepositoryCreatorAction(action)) || seen[action] {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid or duplicate managed creator action %q", action)
		}
		seen[action] = true
	}
	return nil
}

// ValidateShareActions refuses an unknown or duplicated share policy.
func ValidateShareActions(actions []string) error {
	allowed := []string{"object.read", "object.schema.read", "object.provenance", "object.history.read", "object.relations", "dataset.resolve", "file.read"}
	seen := map[string]bool{}
	for _, action := range actions {
		if (!slices.Contains(allowed, action) && !access.IsRepositoryReadAction(action)) || seen[action] {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid or duplicate managed share action %q", action)
		}
		seen[action] = true
	}
	return nil
}

func validManagedUsername(value string) bool {
	_, err := identity.CanonicalUsername(value)
	return err == nil
}

// ValidManagedUsername reports whether a managed store name is a stable key.
func ValidManagedUsername(value string) bool { return validManagedUsername(value) }

// ValidManagedPublicURL reports whether a provisioning panel URL is safe.
func ValidManagedPublicURL(raw string) bool { return validManagedPublicURL(raw) }

func validManagedPublicURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

package home

import (
	"slices"

	"kc/identity"
	"kc/kernel"
)

// AdmissionConfig is an explicit deployment policy for a human's first
// self-service request. Authentication alone never applies this policy.
type AdmissionConfig struct {
	Enabled            bool     `json:"enabled" yaml:"enabled"`
	Catalog            string   `json:"catalog" yaml:"catalog"`
	AuthenticatedUsers bool     `json:"authenticatedUsers,omitempty" yaml:"authenticatedUsers,omitempty"`
	Principals         []string `json:"principals,omitempty" yaml:"principals,omitempty"`
	Actions            []string `json:"actions" yaml:"actions"`
}

// Allows reports policy eligibility only. The service must first verify this
// is the current human's canonical username, not a caller-selected principal.
func (p *AdmissionConfig) Allows(principal string) bool {
	return p != nil && p.Enabled && (p.AuthenticatedUsers || slices.Contains(p.Principals, principal))
}

func validateAdmissionConfig(c DeploymentConfig) error {
	p := c.Admission
	if p == nil {
		return nil
	}
	invalid := func(message string) error { return kernel.Fail(kernel.ErrUsageInvalid, "admission: %s", message) }
	found := false
	for _, cat := range c.Catalogs {
		found = found || cat.ID == p.Catalog
	}
	if !found {
		return invalid("catalog must name a declared Catalog")
	}
	if p.AuthenticatedUsers == (len(p.Principals) > 0) {
		return invalid("choose authenticatedUsers or explicit principals")
	}
	seen := map[string]bool{}
	for _, name := range p.Principals {
		if _, err := identity.CanonicalUsername(name); err != nil || seen[name] {
			return invalid("principals must be unique canonical human usernames")
		}
		seen[name] = true
	}
	if len(p.Actions) == 0 {
		return invalid("actions must be explicit")
	}
	seen = map[string]bool{}
	for _, action := range p.Actions {
		switch action {
		case "catalog.read", "catalog.repositories.create", "catalog.repositories.connect":
		default:
			return invalid("unsupported initial action " + action)
		}
		if seen[action] {
			return invalid("actions must be unique")
		}
		seen[action] = true
	}
	return nil
}

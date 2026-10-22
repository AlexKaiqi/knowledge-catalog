package config

import (
	"net/url"
	"path"
	"slices"
	"strings"

	"kc/kernel"
)

// ConnectionPolicy explicitly delegates origins and initial repository-scoped
// actions. Authentication or connection ownership does not imply these grants.
type ConnectionPolicy struct {
	AllowedOrigins []string `json:"allowedOrigins" yaml:"allowedOrigins"`
	CreatorActions []string `json:"creatorActions" yaml:"creatorActions"`
	ShareActions   []string `json:"shareActions,omitempty" yaml:"shareActions,omitempty"`
}

// ValidateConnectionPolicy refuses a policy that cannot be evaluated safely.
func ValidateConnectionPolicy(policy *ConnectionPolicy) error {
	if policy == nil {
		return nil
	}
	if len(policy.AllowedOrigins) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "connections requires explicit allowedOrigins")
	}
	seen := map[string]bool{}
	for _, origin := range policy.AllowedOrigins {
		u, err := validConnectionURL(origin)
		if err != nil {
			return err
		}
		canonical := strings.TrimRight(u.String(), "/")
		if canonical != origin || seen[origin] {
			return kernel.Fail(kernel.ErrUsageInvalid, "connection origins must be unique canonical provider origins")
		}
		seen[origin] = true
	}
	if err := validateConnectionActions(policy.CreatorActions); err != nil {
		return err
	}
	return ValidateShareActions(policy.ShareActions)
}

func validConnectionURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(raw, "\\\r\n") || (u.Path != "" && path.Clean(u.Path) != strings.TrimRight(u.Path, "/")) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "connection URL must be a canonical http(s) URL without credentials, query, or fragment")
	}
	return u, nil
}

func validateConnectionActions(actions []string) error {
	return ValidateConnectionActions(actions)
}

// ValidateConnectionActions refuses a creator policy that cannot delegate
// connection management explicitly.
func ValidateConnectionActions(actions []string) error {
	if !slices.Contains(actions, "repository.connections.manage") {
		return kernel.Fail(kernel.ErrUsageInvalid, "connection creatorActions must explicitly include repository.connections.manage")
	}
	rest := make([]string, 0, len(actions))
	seen := false
	for _, action := range actions {
		if action == "repository.connections.manage" {
			if seen {
				return kernel.Fail(kernel.ErrUsageInvalid, "duplicate connection creator action")
			}
			seen = true
		} else {
			rest = append(rest, action)
		}
	}
	if len(rest) > 0 {
		return ValidateCreatorActions(rest)
	}
	return nil
}

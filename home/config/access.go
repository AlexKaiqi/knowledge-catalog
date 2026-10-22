package config

import (
	"path/filepath"
	"strings"

	"kc/kernel"
)

// RepositoryAccess is a deployment-declared default for authenticated
// principals on one Repository. It is not a Registry visibility flag, not a
// SEARCH filter key, and not a grant to a synthetic principal.
type RepositoryAccess struct {
	ID                   string   `json:"id" yaml:"id"`
	AuthenticatedActions []string `json:"authenticatedActions,omitempty" yaml:"authenticatedActions,omitempty"`
}

// AuthenticatedRepositoryActions is the closed consume-side set a Repository
// may grant to every authenticated principal. Write, grant, Registry manage
// and dataset administer stay on explicit allow rules.
func AuthenticatedRepositoryActions() []string {
	return []string{
		"object.read",
		"object.schema.read",
		"object.history.read",
		"object.provenance",
		"object.relations",
		"object.access.describe",
		"file.read",
		"projection.read",
	}
}

// ValidateAuthenticatedActions refuses an action outside the closed set.
func ValidateAuthenticatedActions(actions []string) error {
	allowed := map[string]bool{}
	for _, action := range AuthenticatedRepositoryActions() {
		allowed[action] = true
	}
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" || !allowed[action] {
			return kernel.Fail(kernel.ErrUsageInvalid, "authenticatedActions cannot include %q", action)
		}
	}
	return nil
}

// ValidateRepositoryAccess refuses malformed or duplicate declarations.
func ValidateRepositoryAccess(items []RepositoryAccess) error {
	seen := map[string]bool{}
	for _, item := range items {
		id, err := NormalizeRepositoryID(item.ID)
		if err != nil || id != item.ID {
			return kernel.Fail(kernel.ErrUsageInvalid, "repositoryAccess has invalid identity %q", item.ID)
		}
		if seen[id] {
			return kernel.Fail(kernel.ErrUsageInvalid, "repositoryAccess has duplicate identity %q", id)
		}
		seen[id] = true
		if err := ValidateAuthenticatedActions(item.AuthenticatedActions); err != nil {
			return err
		}
	}
	return nil
}

// PathsOverlap reports whether two configured directories nest inside each
// other after resolving symlinks that already exist.
func PathsOverlap(a, b string) bool {
	a, b = realPath(a), realPath(b)
	inside := func(root, p string) bool {
		rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return inside(a, b) || inside(b, a)
}

func realPath(p string) string {
	p = filepath.Clean(p)
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(realPath(parent), filepath.Base(p))
}

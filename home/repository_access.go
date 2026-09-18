package home

import (
	"os"
	"path/filepath"
	"strings"

	"kc/internal/jsonfile"
	"kc/kernel"
	"kc/knowledge"
)

const (
	repositoryAccessVersion = 1
	repositoryAccessFile    = "repository-access.json"
)

// RepositoryAccess is a deployment-declared default for authenticated
// principals on one Repository. It is not a Catalog visibility flag, not a
// SEARCH filter key, and not a grant to a synthetic principal.
type RepositoryAccess struct {
	ID                   string   `json:"id" yaml:"id"`
	AuthenticatedActions []string `json:"authenticatedActions,omitempty" yaml:"authenticatedActions,omitempty"`
}

// RepositoryAccessFile is the durable copy of repositoryAccess for a Home
// or deployment stateDir. Evaluation fail-closes when the file is missing
// or unreadable.
type RepositoryAccessFile struct {
	Version      int                `json:"version"`
	Repositories []RepositoryAccess `json:"repositories"`
}

// AuthenticatedRepositoryActions is the closed consume-side set a Repository
// may grant to every authenticated principal. Write, grant, Catalog manage
// and knowledge-set administer stay on explicit allow rules.
func AuthenticatedRepositoryActions() []string {
	return []string{
		"knowledge.read",
		"knowledge.schema.read",
		"knowledge.search",
		"knowledge.history.read",
		"knowledge.provenance",
		"knowledge.relations",
		"knowledge.access.describe",
		"file.read",
		"projection.read",
	}
}

// SystemRepositoryAccess is the recommended declaration for the protocol
// publication Repository. Init and fixtures write it; the authorizer never
// infers it from the reserved ID.
func SystemRepositoryAccess() RepositoryAccess {
	return RepositoryAccess{
		ID:                   string(knowledge.SystemRepositoryID),
		AuthenticatedActions: AuthenticatedRepositoryActions(),
	}
}

func (c DeploymentConfig) RepositoryAllowsAuthenticated(repo, action string) bool {
	return repositoryAccessAllows(c.RepositoryAccess, repo, action)
}

func (f RepositoryAccessFile) Allows(repo, action string) bool {
	return repositoryAccessAllows(f.Repositories, repo, action)
}

func repositoryAccessAllows(items []RepositoryAccess, repo, action string) bool {
	if repo == "" || action == "" {
		return false
	}
	for _, item := range items {
		if item.ID != repo {
			continue
		}
		for _, granted := range item.AuthenticatedActions {
			if granted == action {
				return true
			}
		}
	}
	return false
}

func repositoryAccessPath(dir string) string {
	return filepath.Join(dir, repositoryAccessFile)
}

func ReadRepositoryAccess(dir string) (RepositoryAccessFile, error) {
	path := repositoryAccessPath(dir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return RepositoryAccessFile{Version: repositoryAccessVersion}, nil
	}
	var file RepositoryAccessFile
	if err := jsonfile.Read(path, &file); err != nil {
		return RepositoryAccessFile{}, err
	}
	if file.Repositories == nil {
		file.Repositories = []RepositoryAccess{}
	}
	file.Version = repositoryAccessVersion
	return file, nil
}

func WriteRepositoryAccess(dir string, file RepositoryAccessFile) error {
	if file.Repositories == nil {
		file.Repositories = []RepositoryAccess{}
	}
	file.Version = repositoryAccessVersion
	return jsonfile.Write(repositoryAccessPath(dir), file)
}

// EnsureRepositoryAccess writes items only when the durable file is missing.
// An existing file — including an empty one — is the operator's policy.
func EnsureRepositoryAccess(dir string, items ...RepositoryAccess) error {
	if fileExists(repositoryAccessPath(dir)) {
		return nil
	}
	file := RepositoryAccessFile{Version: repositoryAccessVersion}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if err := ValidateAuthenticatedActions(item.AuthenticatedActions); err != nil {
			return err
		}
		file.Repositories = append(file.Repositories, item)
	}
	return WriteRepositoryAccess(dir, file)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

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

func validateRepositoryAccess(items []RepositoryAccess) error {
	seen := map[string]bool{}
	for _, item := range items {
		id, err := NormalizeCatalogID(item.ID)
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

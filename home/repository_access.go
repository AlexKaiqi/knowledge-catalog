package home

import (
	"os"
	"path/filepath"

	"kc/home/config"
	"kc/internal/jsonfile"
	objectmodel "kc/object"
)

const (
	repositoryAccessVersion = 1
	repositoryAccessFile    = "repository-access.json"
)

var (
	// AuthenticatedRepositoryActions is the closed consume-side action set.
	AuthenticatedRepositoryActions = config.AuthenticatedRepositoryActions
	// ValidateAuthenticatedActions is the static action contract.
	ValidateAuthenticatedActions = config.ValidateAuthenticatedActions
)

// RepositoryAccessFile is the durable copy of repositoryAccess for a Home
// or deployment stateDir. Evaluation fail-closes when the file is missing
// or unreadable.
type RepositoryAccessFile struct {
	Version      int                `json:"version"`
	Repositories []RepositoryAccess `json:"repositories"`
}

// SystemRepositoryAccess is the recommended declaration for the protocol
// publication Repository. Init and fixtures write it; the authorizer never
// infers it from the reserved ID.
func SystemRepositoryAccess() RepositoryAccess {
	return RepositoryAccess{
		ID:                   string(objectmodel.SystemRepositoryID),
		AuthenticatedActions: AuthenticatedRepositoryActions(),
	}
}

// RepositoryAllowsAuthenticated evaluates one declared default. It is a
// function because DeploymentConfig is owned by home/config.
func RepositoryAllowsAuthenticated(c DeploymentConfig, repo, action string) bool {
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

// ReadRepositoryAccess returns the declared authenticated Repository defaults.
// A missing declaration means none; an unreadable one is an error so callers
// fail closed.
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

// WriteRepositoryAccess persists the declaration for a state directory.
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

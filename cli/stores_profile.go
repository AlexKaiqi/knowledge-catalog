package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s StoresFile) withDefaults() StoresFile {
	if s.Profile == "" {
		s.Profile = "local"
	}
	if s.Repository == "" {
		if s.Profile == "scale" {
			s.Repository = "dolt"
		} else {
			s.Repository = defaultRepositoryDriver
		}
	}
	s.Repository = normalizeRepoDriver(s.Repository)
	if s.Index == "" {
		if s.Profile == "scale" {
			s.Index = "opensearch"
		} else {
			s.Index = defaultIndexDriver
		}
	}
	s.Index = normalizeIndexDriver(s.Index)
	if s.Layout.Repos == "" {
		s.Layout.Repos = defaultReposDir
	}
	if s.Layout.Catalogs == "" {
		s.Layout.Catalogs = defaultCatalogsDir
	}
	if s.Layout.Projections == "" {
		s.Layout.Projections = defaultProjectionsDir
	}
	if s.Layout.Checkouts == "" {
		s.Layout.Checkouts = defaultCheckoutsDir
	}
	return s
}

func (s StoresFile) validateProfile() error {
	switch s.Profile {
	case "", "local", "scale":
	default:
		return fmt.Errorf("unknown store profile %s (want local or scale)", s.Profile)
	}
	if s.Repository == "postgres" {
		return errUnsupportedDriver("repository", s.Repository)
	}
	switch s.Repository {
	case "filegit", "dolt", "gitea":
	default:
		return errUnsupportedDriver("repository", s.Repository)
	}
	switch s.Index {
	case "sqlite", "opensearch":
	default:
		return fmt.Errorf("%s is not a Knowledge Catalog projection provider", s.Index)
	}
	return nil
}

func normalizeRepoDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "filegit", "git", "local":
		return "filegit"
	case "dolt":
		return "dolt"
	case "gitea":
		return "gitea"
	case "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeIndexDriver(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "sqlite", "fts":
		return "sqlite"
	case "opensearch":
		return "opensearch"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeProfile(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "local":
		return "local", nil
	case "scale":
		return "scale", nil
	default:
		return "", fmt.Errorf("unknown store profile %s (want local or scale)", raw)
	}
}

func errUnsupportedDriver(kind, driver string) error {
	if kind == "store" {
		return fmt.Errorf("unknown store driver %s: use a configured repository or projection driver", driver)
	}
	return fmt.Errorf("unknown %s driver %s: snapshot repositories support filegit, dolt, or gitea", kind, driver)
}

// resolveStoreDir joins a layout directory with --home unless it is absolute.
func resolveStoreDir(home, dir, fallback string) (string, error) {
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

func (s StoresFile) rejectSecrets() error {
	if err := s.OpenSearch.RejectSecrets(); err != nil {
		return err
	}
	return s.StarRocks.RejectSecrets()
}

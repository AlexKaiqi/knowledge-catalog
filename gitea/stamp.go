package gitea

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const stampFile = "remote.yaml"

type stamp struct {
	ID     string `yaml:"id"`
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// WriteStamp records how to reopen this Gitea member (no token).
func WriteStamp(dir, repositoryID, dsn string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(stamp{ID: repositoryID, Driver: "gitea", DSN: dsn})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stampFile), b, 0o644)
}

// ReadStamp loads a Gitea pointer directory.
func ReadStamp(dir string) (id, dsn string, err error) {
	b, err := os.ReadFile(filepath.Join(dir, stampFile))
	if err != nil {
		return "", "", err
	}
	var s stamp
	if err := yaml.Unmarshal(b, &s); err != nil {
		return "", "", err
	}
	if s.ID == "" || s.DSN == "" {
		return "", "", fmt.Errorf("invalid gitea stamp in %s", dir)
	}
	return s.ID, s.DSN, nil
}

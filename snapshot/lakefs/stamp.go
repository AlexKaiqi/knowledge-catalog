package lakefs

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

// WriteStamp records only the public location needed to reopen a member.
// Credentials remain in Server-private storage or the deployment environment.
func WriteStamp(dir, repositoryID, dsn string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(stamp{ID: repositoryID, Driver: "lakefs", DSN: dsn})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stampFile), raw, 0o644)
}

func ReadStamp(dir string) (id, dsn string, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, stampFile))
	if err != nil {
		return "", "", err
	}
	var value stamp
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return "", "", err
	}
	if value.Driver != "lakefs" || value.ID == "" || value.DSN == "" {
		return "", "", fmt.Errorf("invalid lakefs stamp in %s", dir)
	}
	return value.ID, value.DSN, nil
}

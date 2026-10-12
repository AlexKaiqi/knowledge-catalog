// Package stamp owns the member pointer directory format shared by Snapshot
// adapters: one remote.yaml recording how to reopen a member. Credentials
// stay in Server-private storage or the deployment environment, never here.
package stamp

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the pointer file every adapter writes into its member directory.
const FileName = "remote.yaml"

type file struct {
	ID     string `yaml:"id"`
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// Write records the public location of one member directory.
func Write(dir, driver, repositoryID, dsn string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(file{ID: repositoryID, Driver: driver, DSN: dsn})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), raw, 0o644)
}

// Read loads a member pointer directory and requires the expected driver.
func Read(dir, driver string) (id, dsn string, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return "", "", err
	}
	var value file
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return "", "", err
	}
	if value.Driver != driver || value.ID == "" || value.DSN == "" {
		return "", "", fmt.Errorf("invalid %s stamp in %s", driver, dir)
	}
	return value.ID, value.DSN, nil
}

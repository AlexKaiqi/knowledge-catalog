// Package starrocks declares the scale layer ③ column projection adapter.
package starrocks

import (
	"fmt"
	"strings"

	"kc/index"
	"kc/kernel"
	"kc/snapshot"
)

// Open is the scale column projection (filter / compare / aggregate).
func Open(_ Config) index.EngineOpener {
	return func(string, kernel.RepositoryID) (index.Engine, error) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "starrocks column projection is not implemented")
	}
}

// Config is non-secret FE location (MySQL protocol). Password is env.
type Config struct {
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
	User     string `json:"user,omitempty" yaml:"user,omitempty"`
	Database string `json:"database,omitempty" yaml:"database,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}

const EnvPassword = "KC_STARROCKS_PASSWORD"

func (c Config) RejectSecrets() error {
	if err := snapshot.RejectConfiguredSecret("starrocks", c.Host, EnvPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("starrocks connection config must not contain secrets; set %s", EnvPassword)
	}
	return nil
}

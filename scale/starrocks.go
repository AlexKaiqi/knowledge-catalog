package scale

import (
	"fmt"
	"strings"

	"kc/index"
	"kc/kernel"
	"kc/repository"
)

// OpenStarRocks is the scale column projection (filter / compare / aggregate).
func OpenStarRocks(_ StarRocksConfig) index.EngineOpener {
	return func(string, kernel.RepositoryID) (index.Engine, error) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "starrocks column projection is not implemented")
	}
}

// StarRocksConfig is non-secret FE location (MySQL protocol). Password is env.
type StarRocksConfig struct {
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
	User     string `json:"user,omitempty" yaml:"user,omitempty"`
	Database string `json:"database,omitempty" yaml:"database,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}

const EnvStarRocksPassword = "KC_STARROCKS_PASSWORD"

func (c StarRocksConfig) RejectSecrets() error {
	if err := repository.RejectConfiguredSecret("starrocks", c.Host, EnvStarRocksPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("starrocks connection config must not contain secrets; set %s", EnvStarRocksPassword)
	}
	return nil
}

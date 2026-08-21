package scale

import (
	"fmt"
	"strings"

	"kc/index"
	"kc/kernel"
	"kc/repository"
)

// OpenStream is the scale APPEND authority (layer ⓪ ordered log). Not a Snapshot.
// Do not repo-add this as a Catalog member. Append itself is not assembled.
func OpenStream(_ string, id kernel.RepositoryID) (repository.Stream, error) {
	return &streamStub{bind: id}, nil
}

type streamStub struct {
	bind kernel.RepositoryID
}

var _ repository.Stream = (*streamStub)(nil)

func (s *streamStub) Append(string, []repository.AppendEntry, string) ([]string, error) {
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "scale append stream is not implemented (id %s)", s.bind)
}

func (s *streamStub) StreamCursor(string) string { return "0" }

func (s *streamStub) ReadStream(streamRef string) repository.StreamSlice {
	return repository.StreamSlice{Repository: s.bind, StreamRef: streamRef, Cursor: "0"}
}

// OpenStarRocks is the scale column index (filter / compare / aggregate).
// Connection belongs in the scene .env, not this package as a Hippo connector.
func OpenStarRocks(_ StarRocksConfig) index.EngineOpener {
	return func(string, kernel.RepositoryID) (index.Engine, error) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "starrocks column index is not implemented")
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

// RejectSecrets refuses passwords in the config file. Password is KC_STARROCKS_PASSWORD.
func (c StarRocksConfig) RejectSecrets() error {
	if err := repository.RejectConfiguredSecret("starrocks", c.Host, EnvStarRocksPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("starrocks connection config must not contain secrets; set %s", EnvStarRocksPassword)
	}
	return nil
}

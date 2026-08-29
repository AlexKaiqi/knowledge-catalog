// Package opensearch implements the OpenSearch scale projection.
package opensearch

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/snapshot"
)

const (
	EnvPassword          = "KC_ELASTICSEARCH_PASSWORD"
	EnvAPIKey            = "KC_ELASTICSEARCH_API_KEY"
	defaultOpenSearchURL = "http://127.0.0.1:9200"
	defaultPrimaryShards = 8
	defaultRefresh       = "1s"
)

// Config is non-secret cluster location for full-text (MATCH).
// This is a derived projection, not authority. Password and API key stay in the environment.
type Config struct {
	URL             string `json:"url,omitempty" yaml:"url,omitempty"`
	User            string `json:"user,omitempty" yaml:"user,omitempty"`
	PrimaryShards   int    `json:"primaryShards,omitempty" yaml:"primaryShards,omitempty"`
	Replicas        int    `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	RefreshInterval string `json:"refreshInterval,omitempty" yaml:"refreshInterval,omitempty"`
	Password        string `json:"password,omitempty" yaml:"password,omitempty"`
	APIKey          string `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
}

func (c Config) WithDefaults() Config {
	if strings.TrimSpace(c.URL) == "" {
		c.URL = defaultOpenSearchURL
	}
	if c.PrimaryShards == 0 {
		c.PrimaryShards = defaultPrimaryShards
	}
	if strings.TrimSpace(c.RefreshInterval) == "" {
		c.RefreshInterval = defaultRefresh
	}
	return c
}

func (c Config) validateScaleSettings() error {
	if c.PrimaryShards < 1 || c.PrimaryShards > 1024 {
		return fmt.Errorf("opensearch primaryShards must be between 1 and 1024")
	}
	if c.Replicas < 0 || c.Replicas > 20 {
		return fmt.Errorf("opensearch replicas must be between 0 and 20")
	}
	return nil
}

func (c Config) RejectSecrets() error {
	if err := snapshot.RejectConfiguredSecret("opensearch", c.URL, EnvPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("opensearch connection config must not contain secrets; set %s", EnvPassword)
	}
	if strings.TrimSpace(c.APIKey) != "" {
		return fmt.Errorf("opensearch connection config must not contain secrets; set %s", EnvAPIKey)
	}
	return nil
}

// Open returns an EngineOpener for one projection per repository. Credentials
// are resolved at open time so they never enter persisted provider config.
func Open(cfg Config) index.EngineOpener {
	cfg = cfg.WithDefaults()
	return func(_ string, id kernel.RepositoryID) (index.Engine, error) {
		if err := cfg.RejectSecrets(); err != nil {
			return nil, err
		}
		if err := cfg.validateScaleSettings(); err != nil {
			return nil, err
		}
		prefix, controlID := projectionNames(id)
		eng := &openSearchEngine{
			base:            strings.TrimRight(cfg.URL, "/"),
			prefix:          prefix,
			controlID:       controlID,
			http:            &http.Client{Timeout: 12 * time.Second},
			user:            cfg.User,
			pass:            strings.TrimSpace(os.Getenv(EnvPassword)),
			apiKey:          strings.TrimSpace(os.Getenv(EnvAPIKey)),
			repository:      id,
			primaryShards:   cfg.PrimaryShards,
			replicas:        cfg.Replicas,
			refreshInterval: cfg.RefreshInterval,
		}
		if err := eng.ensureControlIndex(); err != nil {
			return nil, err
		}
		return eng, nil
	}
}

// Check verifies that the configured service endpoint is reachable and the
// supplied credentials are accepted without creating an index or projection.
func Check(cfg Config) error {
	if err := cfg.RejectSecrets(); err != nil {
		return err
	}
	cfg = cfg.WithDefaults()
	if err := cfg.validateScaleSettings(); err != nil {
		return err
	}
	eng := &openSearchEngine{
		base:   strings.TrimRight(cfg.URL, "/"),
		http:   &http.Client{Timeout: 12 * time.Second},
		user:   cfg.User,
		pass:   strings.TrimSpace(os.Getenv(EnvPassword)),
		apiKey: strings.TrimSpace(os.Getenv(EnvAPIKey)),
	}
	status, body, err := eng.doBytes(http.MethodGet, "/", nil, "")
	if err != nil {
		return err
	}
	if status >= http.StatusBadRequest {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "opensearch readiness returned HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}
	return nil
}

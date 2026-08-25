// Package elasticsearch implements the scale layer ③ text projection.
package elasticsearch

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
	EnvPassword             = "KC_ELASTICSEARCH_PASSWORD"
	EnvAPIKey               = "KC_ELASTICSEARCH_API_KEY"
	defaultElasticsearchURL = "http://127.0.0.1:9200"
)

// Config is non-secret cluster location for full-text (MATCH).
// Not StarRocks, not authority. Password and API key stay in the environment.
type Config struct {
	URL      string `json:"url,omitempty" yaml:"url,omitempty"`
	User     string `json:"user,omitempty" yaml:"user,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	APIKey   string `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
}

func (c Config) WithDefaults() Config {
	if strings.TrimSpace(c.URL) == "" {
		c.URL = defaultElasticsearchURL
	}
	return c
}

func (c Config) RejectSecrets() error {
	if err := snapshot.RejectConfiguredSecret("elasticsearch", c.URL, EnvPassword); err != nil {
		return err
	}
	if strings.TrimSpace(c.Password) != "" {
		return fmt.Errorf("elasticsearch connection config must not contain secrets; set %s", EnvPassword)
	}
	if strings.TrimSpace(c.APIKey) != "" {
		return fmt.Errorf("elasticsearch connection config must not contain secrets; set %s", EnvAPIKey)
	}
	return nil
}

// Open returns an EngineOpener for one projection per repository. Credentials
// are resolved at open time so they never enter persisted provider config.
func Open(cfg Config) index.EngineOpener {
	return func(_ string, id kernel.RepositoryID) (index.Engine, error) {
		if err := cfg.RejectSecrets(); err != nil {
			return nil, err
		}
		cfg = cfg.WithDefaults()
		eng := &esEngine{
			base:       strings.TrimRight(cfg.URL, "/"),
			index:      "kc-proj-" + strings.ToLower(index.SanitizeID(string(id))),
			http:       &http.Client{Timeout: 12 * time.Second},
			user:       cfg.User,
			pass:       strings.TrimSpace(os.Getenv(EnvPassword)),
			apiKey:     strings.TrimSpace(os.Getenv(EnvAPIKey)),
			repository: id,
		}
		if err := eng.ensureIndex(); err != nil {
			return nil, err
		}
		return eng, nil
	}
}

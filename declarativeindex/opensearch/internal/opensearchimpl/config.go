// Package opensearch implements the OpenSearch scale projection.
package opensearchimpl

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"kc/discovery/llmhttp"
	index "kc/discovery/projection"
	"kc/kernel"
	"kc/snapshot"
)

const (
	EnvPassword          = "KC_ELASTICSEARCH_PASSWORD"
	EnvAPIKey            = "KC_ELASTICSEARCH_API_KEY"
	EnvEmbeddingModel    = "KC_EMBEDDING_MODEL"
	EnvEmbeddingDims     = "KC_EMBEDDING_DIMENSIONS"
	defaultOpenSearchURL = "http://127.0.0.1:9200"
	defaultPrimaryShards = 8
	defaultRefresh       = "1s"
)

// Config is non-secret cluster location for full-text (MATCH).
// This is a derived projection, not authority. Password and API key stay in the environment.
type Config struct {
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// Namespace is a stable, non-secret instance identity. Empty adopts legacy resources.
	Namespace       string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
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
	return func(directory string, id kernel.RepositoryID) (index.Engine, error) {
		eng, err := openConfiguredClient(cfg)
		if err != nil {
			return nil, err
		}
		owner := eng.resourceScope(id)
		eng.repository, eng.prefix, eng.controlID = owner.repository, owner.prefix, owner.controlID
		if directory != "" && eng.namespace != "" {
			cache := eng.computationCache(directory)
			eng.computationCacheAuthority, eng.computationCacheDirectory = cache.authority, cache.directory
		}
		// The embedding provider is process-level configuration like the
		// credentials: model identity comes from the environment and never
		// enters persisted provider config. Unset model keeps the projection
		// purely lexical.
		model, dims, err := configuredEmbeddingModel()
		if err != nil {
			return nil, err
		}
		if model != "" {
			embedder, err := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{
				BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"), Model: model, Dimensions: dims,
			})
			if err != nil {
				return nil, err
			}
			eng.embedder = embedder
			eng.vectorDimension = dims
			eng.vectorModel = model

			eng.embeddingIdentity = string(kernel.CanonicalDigest([]any{"llmhttp-embeddings-v1", strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "/"), model, dims}))
			eng.embeddingCacheIdentity = string(kernel.CanonicalDigest([]any{eng.embeddingIdentity, kernel.CanonicalDigest(strings.TrimSpace(os.Getenv("OPENAI_API_KEY")))}))
		}
		// Opening a read handle is local. Network I/O is performed by the
		// context-aware read methods or the explicit maintenance lifecycle.
		return eng, nil
	}
}

// openConfiguredClient carries no logical query scope and performs no IO.
// Query engines and the instance resource coordinator share these exact
// namespace and credential coordinates without constructing a fictitious Repo.
func openConfiguredClient(cfg Config) (*openSearchEngine, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.RejectSecrets(); err != nil {
		return nil, err
	}
	if err := cfg.validateScaleSettings(); err != nil {
		return nil, err
	}
	controlIndex := controlIndexName
	if cfg.Namespace != "" {
		controlIndex += "-" + string(namespaceDigest(cfg.Namespace))[:24]
	}
	return &openSearchEngine{base: strings.TrimRight(cfg.URL, "/"), controlIndex: controlIndex, namespace: cfg.Namespace,
		http: &http.Client{Timeout: 12 * time.Second}, user: cfg.User,
		pass: strings.TrimSpace(os.Getenv(EnvPassword)), apiKey: strings.TrimSpace(os.Getenv(EnvAPIKey)),
		primaryShards: cfg.PrimaryShards, replicas: cfg.Replicas, refreshInterval: cfg.RefreshInterval}, nil
}

// ConfiguredEmbeddingModels reports only models whose engine configuration can
// create the same pinned vector dimension. It performs no network call.
func ConfiguredEmbeddingModels() ([]string, error) {
	model, _, err := configuredEmbeddingModel()
	if err != nil {
		return nil, err
	}
	if model == "" {
		return nil, nil
	}
	return []string{model}, nil
}

func configuredEmbeddingModel() (string, int, error) {
	model := strings.TrimSpace(os.Getenv(EnvEmbeddingModel))
	if model == "" {
		return "", 0, nil
	}
	dims, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvEmbeddingDims)))
	if err != nil || dims < 1 || dims > 16384 {
		return "", 0, kernel.Fail(kernel.ErrUsageInvalid, "%s requires %s between 1 and 16384", EnvEmbeddingModel, EnvEmbeddingDims)
	}
	return model, dims, nil
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

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"kc/home/config"
)

// Reading and validating a deployment document is static: it reads no source,
// opens no engine, writes no deployment state and starts no maintenance. The
// strongest available evidence is that a document pointing at an unreachable
// authority still validates, and that validation leaves the state directory
// untouched.
func TestReadAndValidateDeploymentNeverTouchesItsSources(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	cache := filepath.Join(t.TempDir(), "cache")
	document := filepath.Join(t.TempDir(), "deployment.json")
	body := `{
  "version": 1,
  "stateDir": "` + state + `",
  "cacheDir": "` + cache + `",
  "auth": "local",
  "registry": {"id": "kr://platform/catalog", "driver": "lakefs", "dsn": "http://127.0.0.1:1/platform-catalog"},
  "repositories": [{"id": "kr://kc/system", "driver": "lakefs", "dsn": "http://127.0.0.1:1/system"}],
  "bootstrapPrincipal": "system:fixture"
}`
	if err := os.WriteFile(document, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := config.ReadDeployment(document)
	if err != nil {
		t.Fatalf("a statically valid document must not require a reachable authority: %v", err)
	}
	if err := read.Validate(); err != nil {
		t.Fatalf("validate is static and must not contact the authority: %v", err)
	}
	for _, dir := range []string{state, cache} {
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			t.Fatalf("configuration created %s; reading a document must not write deployment state", dir)
		}
	}
}

// Validation rejects unknown authenticators, and the accepted names are the
// builtin set. That set is only visible because the configuration package
// itself links the builtin authenticators; without that link every shipped
// name would be rejected.
func TestValidateAcceptsExactlyTheRegisteredAuthenticators(t *testing.T) {
	base := config.DeploymentConfig{
		Version:            1,
		StateDir:           filepath.Join(t.TempDir(), "state"),
		CacheDir:           filepath.Join(t.TempDir(), "cache"),
		Auth:               "local",
		BootstrapPrincipal: "system:fixture",
		Registry:           config.RegistryBinding{ID: "kr://platform/catalog", Driver: "lakefs", DSN: "http://127.0.0.1:1/platform-catalog"},
		Repositories:       []config.RepositoryBinding{{ID: "kr://kc/system", Driver: "lakefs", DSN: "http://127.0.0.1:1/system"}},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("builtin authenticator rejected: %v", err)
	}
	for _, unknown := range []string{"lakefs", "gitea", "absent"} {
		candidate := base
		candidate.Auth = unknown
		if err := candidate.Validate(); err == nil {
			t.Fatalf("unregistered authenticator %q accepted", unknown)
		}
	}
}

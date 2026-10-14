package testfixture

import (
	"kc/declarativeindex/opensearch"
	ports "kc/discovery/projection"
	"os"
	"strings"
	"testing"
)

// LiveEngineOpener binds the existing environment and never starts a service.
func LiveEngineOpener(t *testing.T) ports.EngineOpener {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" {
		t.Fatal("index contract tests require real OpenSearch; run them through make test")
	}
	return opensearch.Open(opensearch.Config{URL: endpoint, PrimaryShards: 1})
}

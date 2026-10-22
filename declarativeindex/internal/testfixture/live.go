package testfixture

import (
	"kc/declarativeindex/opensearch"
	ports "kc/discovery/projection"
	"kc/kernel"
	"os"
	"strings"
	"sync"
	"testing"
)

// liveNamespaces maps one test to the namespace it owns. A test may open the
// engine more than once to simulate a reopen, and those handles must address
// the same indices; allocating a namespace per call would make a reopen look
// like an empty cluster.
var liveNamespaces sync.Map

// LiveEngineOpener binds the existing environment and never starts a service.
//
// Every invocation owns a unique namespace, so the projection indices it
// creates are removed when the test ends. Sharing the empty namespace instead
// adopts the legacy resources: those indices are never deleted, and each
// projection adds a shard to a cluster that rejects creates past its shard
// limit. A leaked namespace therefore turns later runs into failures rather
// than slow runs, which is why ownership is allocated here and not by callers.
func LiveEngineOpener(t *testing.T) ports.EngineOpener {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" {
		t.Fatal("index contract tests require real OpenSearch; run them through make test")
	}
	return opensearch.Open(opensearch.Config{URL: endpoint, Namespace: LiveNamespace(t, endpoint), PrimaryShards: 1})
}

// LiveNamespace returns the namespace owned by this test and registers its
// cleanup once. Repeated calls return the same value so that reopening a
// projection sees what the earlier handle wrote. Callers that build their own
// Config use it to keep the same ownership contract as LiveEngineOpener.
func LiveNamespace(t *testing.T, endpoint string) string {
	t.Helper()
	if existing, ok := liveNamespaces.Load(t); ok {
		return existing.(string)
	}
	namespace := "live-" + string(kernel.CanonicalDigest(t.TempDir()))[:24]
	liveNamespaces.Store(t, namespace)
	t.Cleanup(func() {
		CleanupNamespace(t, endpoint, namespace)
		liveNamespaces.Delete(t)
	})
	return namespace
}

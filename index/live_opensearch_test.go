package index_test

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/index"
	"kc/internal/testkit"
	"kc/retrieval/opensearch"
)

var indexTestSequence atomic.Uint64

func liveIndex(t *testing.T) *index.Index {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" {
		t.Fatal("index contract tests require real OpenSearch; run them through make test")
	}
	idx := index.NewIndexEngine("", opensearch.Open(opensearch.Config{URL: endpoint, PrimaryShards: 1}))
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func makeIndexRepository(t *testing.T, base string) *testkit.KnowledgeRepository {
	t.Helper()
	return testkit.MakeRepository(t, uniqueIndexRepositoryID(base))
}

func uniqueIndexRepositoryID(base string) string {
	return fmt.Sprintf("%s/index-test-%d-%d", strings.TrimRight(base, "/"), time.Now().UnixNano(), indexTestSequence.Add(1))
}

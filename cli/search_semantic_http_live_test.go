package cli_test

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/retrieval/llmhttp"
)

// semanticFakeVector mirrors the deterministic bag-of-tokens embedder used by
// the index-package live tests: identical token sets land in identical
// vectors, so "refund timeout diagnosis" outranks unrelated bodies for the
// refund query. One fixed dimension keeps the fake provider wire-compatible.
func semanticFakeVector(text string, dimension int) []float32 {
	vector := make([]float32, dimension)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(token))
		vector[int(hash.Sum32())%dimension] += 1
	}
	return vector
}

// fakeEmbeddingsEndpoint serves the OpenAI-compatible /embeddings wire
// format so the deployment path (HTTP server embedder plus engine embedder,
// both constructed from the same environment) runs without a real model.
func fakeEmbeddingsEndpoint(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, 0, len(payload.Input))
		for i, text := range payload.Input {
			data = append(data, map[string]any{"index": i, "embedding": semanticFakeVector(text, 64)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data, "model": payload.Model})
	}))
	t.Cleanup(server.Close)
	return server
}

// TestHTTPDatasetSemanticRecallServesMemberVectorWindows drives the full
// deployment path: environment-declared embedding capability, the real
// OpenSearch vector projection, the dataset channel semantic fan-out, and
// the approximate envelope — through the typed HTTP API of a running server.
func TestHTTPDatasetSemanticRecallServesMemberVectorWindows(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test-e2e")
	}
	embeddings := fakeEmbeddingsEndpoint(t)
	// The engine-side embedder is built from this environment when the
	// vector projection opens; the server-side provider uses the same
	// endpoint, so both sides share one vector space.
	t.Setenv("KC_EMBEDDING_MODEL", "fake-embed-1")
	t.Setenv("KC_EMBEDDING_DIMENSIONS", "64")
	t.Setenv("OPENAI_BASE_URL", embeddings.URL)
	t.Setenv("OPENAI_API_KEY", "test-only")

	home := testkit.TempDir(t)
	catalog := "kr://acme/catalog"
	repository := "kr://acme/public/semantic-http"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", catalog))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repository)
	body(t, kc(home, "put", "--command-id", "semantic-schema", "--repo", repository,
		"--object", "schema/runbook.semantic", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(home, "put", "--command-id", "semantic-p1", "--repo", repository,
		"--object", "runbook/refund", "--schema-ref", "schema/runbook.semantic", "--value", `{"body":"refund timeout diagnosis"}`))
	body(t, kc(home, "put", "--command-id", "semantic-p2", "--repo", repository,
		"--object", "runbook/deploy", "--schema-ref", "schema/runbook.semantic", "--value", `{"body":"deployment checklist"}`))
	body(t, kc(home, "dataset", "define", "--dataset", workspace, "--revision", "1", "--source", repository+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:semantic-http", "--cmd", "read-workspace", "--catalog", catalog, "--dataset", workspace))
	body(t, kc(home, "allow", "--principal", "agent:semantic-http", "--action", "knowledge.read,knowledge.search", "--repo", repository))
	syncIndexes(t, home, repository)

	// Same constructor the kc serve entry applies to the declared
	// environment (see verbServe).
	embedder, embedderErr := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{
		BaseURL: embeddings.URL, APIKey: "test-only", Model: "fake-embed-1",
	})
	if embedderErr != nil {
		t.Fatal(embedderErr)
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Embedder: embedder})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	status, response := semanticHTTPAs(t, server, "/knowledge/v1/search", "agent:semantic-http", map[string]any{
		"dataset": workspace, "query": "refund timeout", "limit": 2, "recall": "semantic",
	})
	if status != http.StatusOK {
		t.Fatalf("semantic search status=%d response=%#v", status, response)
	}
	completeness, _ := response["completeness"].(string)
	if completeness != "partial" {
		t.Fatalf("semantic recall must answer approximate (partial), got %#v", response)
	}
	hits, _ := response["hits"].([]any)
	if len(hits) == 0 {
		t.Fatalf("semantic recall returned no hits: %#v", response)
	}
	first, _ := hits[0].(map[string]any)
	knowledge, _ := first["knowledge"].(map[string]any)
	address, _ := knowledge["address"].(map[string]any)
	object, _ := address["objectId"].(string)
	if object != "runbook/refund" {
		t.Fatalf("nearest neighbor must rank runbook/refund first, got %q", object)
	}
	claims := fmt.Sprint(response["claims"])
	if !strings.Contains(claims, "approximate") || !strings.Contains(claims, "fake-embed-1") {
		t.Fatalf("claims must disclose the approximate window and model: %s", claims)
	}
	// Lane evidence must disclose the approximate semantic lane.
	evidence, _ := first["evidence"].([]any)
	if len(evidence) == 0 {
		t.Fatalf("hit carries no lane evidence: %#v", first)
	}
	lane, _ := evidence[0].(map[string]any)
	if lane["lane"] != "semantic-vector" || lane["guarantee"] != "approximate" {
		t.Fatalf("semantic lane evidence mismatch: %#v", lane)
	}

	// Lexical recall on the same request stays exact and unchanged.
	status, lexical := semanticHTTPAs(t, server, "/knowledge/v1/search", "agent:semantic-http", map[string]any{
		"dataset": workspace, "query": "refund timeout", "limit": 2, "recall": "lexical",
	})
	if status != http.StatusOK {
		t.Fatalf("lexical search status=%d response=%#v", status, lexical)
	}
	if lexicalCompleteness, _ := lexical["completeness"].(string); lexicalCompleteness != "complete" {
		t.Fatalf("lexical recall must stay complete: %#v", lexical)
	}

	// Without a server-side provider, semantic recall fails closed instead
	// of downgrading to lexical recall.
	plainHandler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if plainCloser, ok := plainHandler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = plainCloser.Close() })
	}
	plainServer := httptest.NewServer(plainHandler)
	t.Cleanup(plainServer.Close)
	status, failClosed := semanticHTTPAs(t, plainServer, "/knowledge/v1/search", "agent:semantic-http", map[string]any{
		"dataset": workspace, "query": "refund timeout", "limit": 2, "recall": "semantic",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("fail-closed status=%d response=%#v", status, failClosed)
	}
	fault, _ := failClosed["error"].(map[string]any)
	if code, _ := fault["code"].(string); code != "CAPABILITY_UNSATISFIED" {
		t.Fatalf("semantic without provider must be CAPABILITY_UNSATISFIED: %#v", failClosed)
	}
}

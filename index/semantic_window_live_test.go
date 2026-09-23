package index_test

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledgeapp"
	"kc/retrieval"
	"kc/snapshot"
)

// fakeVectorFor derives a deterministic bag-of-token vector so the fake
// embeddings service and the test agree on the vector space without a real
// model. Documents whose text shares tokens with the query rank first.
func fakeVectorFor(text string, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(token))
		vector[hash.Sum32()%uint32(dimensions)] = 1
	}
	return vector
}

func newFakeEmbeddingServer(dimensions int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, 0, len(body.Input))
		for i, text := range body.Input {
			data = append(data, map[string]any{"index": i, "embedding": fakeVectorFor(text, dimensions)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": body.Model, "data": data})
	}))
}

func TestSemanticWindowRecallApproximateEnvelopeAndFailClosed(t *testing.T) {
	const dimensions = 64
	repo := makeIndexRepository(t, "kr://acme/public/semantic-window")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "tested runbook"}, "")[0],
		testkit.PutEntity("policy/P-2", map[string]any{"body": "quarterly revenue report"}, "")[0],
	})

	// An engine without an embedding provider owns no vector surface: the
	// window fails closed instead of downgrading to lexical recall. This
	// assertion runs before any embedding environment exists.
	noEmbedderIndex := liveIndex(t)
	t.Cleanup(func() { _ = noEmbedderIndex.Close() })
	if _, err := noEmbedderIndex.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	queryVector := fakeVectorFor("tested runbook", dimensions)
	if _, err := noEmbedderIndex.SemanticWindowAtContext(context.Background(), repo, head,
		retrieval.SearchOf(retrieval.SearchMATCH("tested runbook")), queryVector); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("lexical-only projection must fail closed, got %v", err)
	}

	embedServer := newFakeEmbeddingServer(dimensions)
	t.Cleanup(embedServer.Close)
	t.Setenv("KC_EMBEDDING_MODEL", "fake-embed-1")
	t.Setenv("KC_EMBEDDING_DIMENSIONS", fmt.Sprint(dimensions))
	t.Setenv("OPENAI_BASE_URL", embedServer.URL)
	t.Setenv("OPENAI_API_KEY", "test-key")

	projection := liveIndex(t)
	t.Cleanup(func() { _ = projection.Close() })
	if _, err := projection.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}

	result, err := projection.SemanticWindowAtContext(context.Background(), repo, head,
		retrieval.SearchOf(retrieval.SearchMATCH("tested runbook")), queryVector)
	if err != nil {
		t.Fatal(err)
	}
	if result.Completeness != retrieval.CompletenessPartial {
		t.Fatalf("semantic window must be partial, got %s", result.Completeness)
	}
	if len(result.Hits) == 0 {
		t.Fatal("semantic window returned no hits")
	}
	if result.Hits[0].Knowledge.Address.ObjectID != "policy/P-1" {
		t.Fatalf("nearest hit = %s, want policy/P-1", result.Hits[0].Knowledge.Address.ObjectID)
	}
	for _, hit := range result.Hits {
		if hit.Knowledge.Commit != head {
			t.Fatalf("hit hydrated at wrong basis: %#v", hit.Knowledge)
		}
	}

	// A dimension mismatch must fail closed, not improvise.
	if _, err := projection.SemanticWindowAtContext(context.Background(), repo, head,
		retrieval.SearchOf(retrieval.SearchMATCH("tested runbook")), make([]float32, dimensions-1)); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("dimension mismatch must be USAGE_INVALID, got %v", err)
	}
}

type semanticTestLookup struct{ repo knowledge.Repository }

func (l semanticTestLookup) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (knowledge.Repository, error) {
	if id != l.repo.ID() {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing repository")
	}
	return l.repo, nil
}

type fakeVectorEmbedder struct{ dimensions int }

func (e fakeVectorEmbedder) Embed(_ context.Context, request retrieval.EmbeddingRequest) (retrieval.EmbeddingResult, error) {
	vectors := make([][]float32, len(request.Texts))
	for i, text := range request.Texts {
		vectors[i] = fakeVectorFor(text, e.dimensions)
	}
	return retrieval.EmbeddingResult{Vectors: vectors, Provider: "test", Model: "fake-embed-1", Dimensions: e.dimensions}, nil
}

func TestSemanticExecutorEndToEndOverVectorProjection(t *testing.T) {
	const dimensions = 64
	embedServer := newFakeEmbeddingServer(dimensions)
	t.Cleanup(embedServer.Close)
	t.Setenv("KC_EMBEDDING_MODEL", "fake-embed-1")
	t.Setenv("KC_EMBEDDING_DIMENSIONS", fmt.Sprint(dimensions))
	t.Setenv("OPENAI_BASE_URL", embedServer.URL)
	t.Setenv("OPENAI_API_KEY", "test-key")

	repo := makeIndexRepository(t, "kr://acme/public/semantic-executor")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "tested runbook"}, "")[0],
		testkit.PutEntity("policy/P-2", map[string]any{"body": "quarterly revenue report"}, "")[0],
	})
	projection := liveIndex(t)
	t.Cleanup(func() { _ = projection.Close() })
	if _, err := projection.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	executor := knowledgeapp.SearchExecutor{
		Repositories: semanticTestLookup{repo: repo}, Projection: projection,
		Embedder: fakeVectorEmbedder{dimensions: dimensions},
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("tested runbook"))
	query.Recall = retrieval.RecallSemantic
	result, err := executor.Execute(context.Background(), knowledgeapp.SearchRequest{Repository: repo.ID(), Commit: head, Query: query})
	if err != nil {
		t.Fatal(err)
	}
	if result.Completeness != retrieval.CompletenessPartial || len(result.Hits) == 0 || result.Hits[0].Knowledge.Address.ObjectID != "policy/P-1" {
		t.Fatalf("semantic executor result: completeness=%s hits=%d", result.Completeness, len(result.Hits))
	}
	claims := strings.Join(result.Claims, "\n")
	if !strings.Contains(claims, "approximate k-NN ranking window") || !strings.Contains(claims, "fake-embed-1") {
		t.Fatalf("claims must disclose window and model: %q", claims)
	}
}

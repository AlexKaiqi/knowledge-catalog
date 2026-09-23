package llmhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
	"kc/retrieval"
	"kc/retrieval/llmhttp"
)

func embeddingsRequest() retrieval.EmbeddingRequest {
	return retrieval.EmbeddingRequest{Texts: []string{"gross merchandise value", "refund window"}}
}

func TestEmbeddingsProviderSendsOneBatchedRequestInOrder(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "embed-1" || len(body.Input) != 2 || body.Input[0] != "gross merchandise value" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"embed-1","data":[
			{"index":1,"embedding":[0.4,0.5]},
			{"index":0,"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()
	provider, err := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{BaseURL: server.URL + "/v1", APIKey: "secret", Model: "embed-1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Embed(context.Background(), embeddingsRequest())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("embedding calls = %d, want 1", calls)
	}
	if len(result.Vectors) != 2 || result.Vectors[0][0] != 0.1 || result.Vectors[1][1] != 0.5 {
		t.Fatalf("vectors = %#v, want request order", result.Vectors)
	}
	if result.Model != "embed-1" || result.Dimensions != 2 || result.Provider != "llmhttp" {
		t.Fatalf("result = %#v", result)
	}
}

func TestEmbeddingsProviderFailsClosedOnShortBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"embed-1","data":[{"index":0,"embedding":[0.1]}]}`))
	}))
	defer server.Close()
	provider, _ := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{BaseURL: server.URL, APIKey: "k", Model: "embed-1"})
	_, err := provider.Embed(context.Background(), embeddingsRequest())
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("short batch must fail closed, got %v", err)
	}
}

func TestEmbeddingsProviderFailsClosedOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota", http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider, _ := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{BaseURL: server.URL, APIKey: "k", Model: "embed-1"})
	_, err := provider.Embed(context.Background(), retrieval.EmbeddingRequest{Texts: []string{"x"}})
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("HTTP rejection must be CAPABILITY_UNSATISFIED, got %v", err)
	}
}

func TestEmbeddingsProviderRequiresConfiguration(t *testing.T) {
	if _, err := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{BaseURL: "", APIKey: "k", Model: "m"}); err == nil {
		t.Fatal("missing base URL must be rejected")
	}
	provider, err := llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{BaseURL: "http://localhost", APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Embed(context.Background(), retrieval.EmbeddingRequest{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("empty request must be USAGE_INVALID, got %v", err)
	}
}

package llmhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/kernel"
	"kc/retrieval"
)

// EmbeddingsConfig is an explicit embeddings-compatible provider
// configuration. APIKey is process-only configuration and is never copied
// into requests, results, evidence or diagnostics.
type EmbeddingsConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// EmbeddingsProvider speaks the OpenAI-compatible /embeddings protocol for
// the request-time query vector. Projection-time document vectors flow
// through the same provider contract; batching belongs to one request, never
// to a silent retry loop.
type EmbeddingsProvider struct {
	endpoint string
	apiKey   string
	model    string
	timeout  time.Duration
	client   *http.Client
}

func NewEmbeddings(config EmbeddingsConfig) (*EmbeddingsProvider, error) {
	baseURL := strings.TrimSpace(config.BaseURL)
	apiKey := strings.TrimSpace(config.APIKey)
	model := strings.TrimSpace(config.Model)
	if baseURL == "" || apiKey == "" || model == "" {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "embedding provider requires base URL, API key and model")
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/embeddings")
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "embedding provider base URL is invalid: %v", err)
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &EmbeddingsProvider{endpoint: parsed.String(), apiKey: apiKey, model: model, timeout: timeout, client: client}, nil
}

type embeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingsResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed sends all texts in one request and returns vectors in request order.
// It never retries: a retried call would pay twice for the same vectors.
func (p *EmbeddingsProvider) Embed(ctx context.Context, request retrieval.EmbeddingRequest) (retrieval.EmbeddingResult, error) {
	if p == nil {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "embedding provider is not configured")
	}
	if len(request.Texts) == 0 {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrUsageInvalid, "embedding request requires at least one text")
	}
	for i, text := range request.Texts {
		if strings.TrimSpace(text) == "" {
			return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrUsageInvalid, "embedding request text %d is empty", i)
		}
	}
	payload, err := json.Marshal(embeddingsRequest{Model: p.model, Input: append([]string(nil), request.Texts...)})
	if err != nil {
		return retrieval.EmbeddingResult{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(callCtx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return retrieval.EmbeddingResult{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "embedding provider unreachable: %v", err)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes))
	if err != nil {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "embedding provider response unreadable: %v", err)
	}
	if httpResponse.StatusCode >= 400 {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "embedding provider rejected the request: %s", truncateForError(body))
	}
	var response embeddingsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "embedding provider response is not embeddings-compatible")
	}
	if len(response.Data) != len(request.Texts) {
		return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"embedding provider returned %d vectors for %d texts", len(response.Data), len(request.Texts))
	}
	vectors := make([][]float32, len(request.Texts))
	dimensions := 0
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= len(vectors) || vectors[item.Index] != nil {
			return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "embedding provider returned a duplicate or out-of-range vector index")
		}
		if dimensions == 0 {
			dimensions = len(item.Embedding)
		}
		if len(item.Embedding) != dimensions {
			return retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "embedding provider returned inconsistent dimensions")
		}
		vectors[item.Index] = item.Embedding
	}
	model := response.Model
	if model == "" {
		model = p.model
	}
	return retrieval.EmbeddingResult{Vectors: vectors, Provider: "llmhttp", Model: model, Dimensions: dimensions}, nil
}

// maxResponseBytes bounds one embeddings response; vectors are dense floats so
// a legitimate batch stays far below this ceiling.
const maxResponseBytes = 16 << 20

func truncateForError(body []byte) string {
	if len(body) > maxErrorBytes {
		return fmt.Sprintf("%s...", body[:maxErrorBytes])
	}
	return string(body)
}

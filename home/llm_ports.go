package home

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	retrieval "kc/discovery"
	"kc/discovery/llmhttp"
)

// The LLM ports bind concrete provider clients at assembly (CAP-ORG-01 A-01,
// same pattern as readiness_ports.go): only the assembly root imports concrete
// adapters. The serve surface keeps flag/env semantics (which model, which
// timeout) and receives bound constructors, not provider packages.

// NewRerankerProvider binds the HTTP rerank provider for the declared model.
// Endpoint and credentials come from the deployment environment; they are not
// carried through flags or Schema.
func NewRerankerProvider(model string, timeout time.Duration) (retrieval.Reranker, error) {
	return llmhttp.New(llmhttp.Config{
		BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"),
		Model: model, Timeout: timeout,
	})
}

// NewEmbedderProvider binds the HTTP embeddings provider for the declared
// model, sourced from the same deployment environment.
func NewEmbedderProvider(model string) (retrieval.Embedder, error) {
	dimensions, err := embeddingDimensions()
	if err != nil {
		return nil, err
	}
	return llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{
		BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"),
		Model: model, Dimensions: dimensions,
	})
}

func embeddingDimensions() (int, error) {
	raw := strings.TrimSpace(os.Getenv("KC_EMBEDDING_DIMENSIONS"))
	if raw == "" {
		return 0, nil
	}
	dimensions, err := strconv.Atoi(raw)
	if err != nil || dimensions < 1 || dimensions > 16384 {
		return 0, fmt.Errorf("KC_EMBEDDING_DIMENSIONS requires an integer between 1 and 16384")
	}
	return dimensions, nil
}

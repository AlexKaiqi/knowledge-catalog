package home

import (
	"os"
	"time"

	"kc/retrieval"
	"kc/retrieval/llmhttp"
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
	return llmhttp.NewEmbeddings(llmhttp.EmbeddingsConfig{
		BaseURL: os.Getenv("OPENAI_BASE_URL"), APIKey: os.Getenv("OPENAI_API_KEY"),
		Model: model,
	})
}

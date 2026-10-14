// Package opensearch is the public registration shim for the concrete
// OpenSearch projection engine (CAP-ORG-01 B12, ruling C: the adapter belongs
// to the index capability, the ③ projection). The implementation lives in
// declarativeindex/opensearch/internal/opensearchimpl; only this adapter may import
// it, and the plugin Entry uses this public adapter.
package opensearch

import "kc/declarativeindex/opensearch/internal/opensearchimpl"

type (
	// Config is one OpenSearch connection configuration.
	Config = opensearchimpl.Config
)

const (
	EnvAPIKey         = opensearchimpl.EnvAPIKey
	EnvEmbeddingDims  = opensearchimpl.EnvEmbeddingDims
	EnvEmbeddingModel = opensearchimpl.EnvEmbeddingModel
	EnvPassword       = opensearchimpl.EnvPassword
)

var (
	Check = opensearchimpl.Check
	Open  = opensearchimpl.Open
)

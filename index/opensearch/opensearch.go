// Package opensearch is the public registration shim for the concrete
// OpenSearch projection engine (CAP-ORG-01 B12, ruling C: the adapter belongs
// to the index capability, the ③ projection). The implementation lives in
// index/internal/opensearchimpl; only this shim plus the index tree may import
// it, and assembly roots (home) keep this stable path.
package opensearch

import "kc/index/internal/opensearchimpl"

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

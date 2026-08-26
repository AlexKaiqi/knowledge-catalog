// Package opensearch exposes the OpenSearch layer-③ managed projection.
// The historical retrieval/elasticsearch import path remains as a source
// compatibility shim while configuration migrates to the OpenSearch name.
package opensearch

import (
	"kc/index"
	"kc/retrieval/elasticsearch"
)

const (
	EnvPassword = elasticsearch.EnvPassword
	EnvAPIKey   = elasticsearch.EnvAPIKey
)

type Config = elasticsearch.Config

func Open(cfg Config) index.EngineOpener { return elasticsearch.Open(cfg) }

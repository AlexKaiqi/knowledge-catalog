package index

import (
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

// CompiledDoc is one object extracted from AccessHints. Schema objects never appear.
type CompiledDoc struct {
	ObjectID kernel.ObjectID
	Text     string
	Fields   [][2]string
}

// Meta is projection basis stored by an Engine.
type Meta struct {
	Basis  kernel.CommitID
	Digest kernel.Digest
	Mode   string
	Cause  string
}

// Engine is layer ③: a physical working projection (not authority, not Catalog).
// Local: SQLite FTS + filter columns. Scale: Elasticsearch MATCH; StarRocks column index.
// Schema must not name the engine.
type Engine interface {
	LoadMeta() (Meta, error)
	Rebuild(docs []CompiledDoc, meta Meta) error
	Apply(upserts []CompiledDoc, deletes []kernel.ObjectID, meta Meta) error
	Search(req reader.SearchRequest, spec reader.IndexSpec) ([]kernel.ObjectID, error)
	Count() (int, error)
	Close() error
}

// EngineOpener builds one projection engine for a repository id.
type EngineOpener func(dir string, id kernel.RepositoryID) (Engine, error)

func compileValue(repo repository.Repository, value repository.KnowledgeValue, spec reader.IndexSpec) (CompiledDoc, bool) {
	if kernel.IsSchemaObject(value.Address.ObjectID) {
		return CompiledDoc{}, false
	}
	bound := boundSpec(repo, value, spec)
	return CompiledDoc{
		ObjectID: value.Address.ObjectID,
		Text:     documentText(value, bound),
		Fields:   indexedFields(value, bound),
	}, true
}

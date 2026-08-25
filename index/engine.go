package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

// CompiledDoc is one object extracted from AccessHints. Schema objects never appear.
type CompiledDoc struct {
	ObjectID knowledge.ObjectID
	Text     string
	Fields   [][2]string
}

// Meta is projection basis stored by an Engine.
type Meta struct {
	Basis            kernel.CommitID
	AccessDigest     kernel.Digest
	PhysicalDigest   kernel.Digest
	ProviderRevision string
	Mode             string
	Cause            string
}

type Guarantee string

const (
	GuaranteeExact       Guarantee = "exact"
	GuaranteeSuperset    Guarantee = "superset"
	GuaranteeApproximate Guarantee = "approximate"
	GuaranteeUnsupported Guarantee = "unsupported"
)

type Capability struct {
	Guarantee Guarantee `json:"guarantee"`
	Coverage  float64   `json:"coverage"`
	Reason    string    `json:"reason,omitempty"`
}

type CandidateRef struct {
	Repository kernel.RepositoryID   `json:"repository"`
	ObjectID   knowledge.ObjectID    `json:"objectId"`
	Basis      kernel.CommitID       `json:"basis"`
	Evidence   []reader.LaneEvidence `json:"evidence"`
}

type CandidatePage struct {
	Candidates   []CandidateRef `json:"candidates"`
	Continuation string         `json:"continuation,omitempty"`
	Exhausted    bool           `json:"exhausted"`
}

type RetrieveRequest struct {
	Search       reader.SearchRequest `json:"search"`
	Spec         reader.AccessSpec    `json:"spec"`
	Continuation string               `json:"continuation,omitempty"`
}

// Retriever locates typed candidates. It never returns knowledge payload.
type Retriever interface {
	Probe(reader.SearchClause, reader.AccessSpec) Capability
	Retrieve(RetrieveRequest) (CandidatePage, error)
}

// ProjectionMaintainer owns only discardable physical projection state.
type ProjectionMaintainer interface {
	LoadMeta() (Meta, error)
	Rebuild(docs []CompiledDoc, meta Meta) error
	Apply(upserts []CompiledDoc, deletes []knowledge.ObjectID, meta Meta) error
	Count() (int, error)
}

type ProviderIdentity interface {
	ProviderID() string
	ProviderRevision() string
	PhysicalDigest() kernel.Digest
}

// Engine is the managed-projection adapter used by this Snapshot reference
// implementation. A source-pushdown provider may implement Retriever only.
type Engine interface {
	Retriever
	ProjectionMaintainer
	Close() error
}

// EngineOpener builds one projection engine for a repository id.
type EngineOpener func(dir string, id kernel.RepositoryID) (Engine, error)

func compileValue(repo knowledge.Repository, value knowledge.KnowledgeValue, spec reader.AccessSpec) (CompiledDoc, bool) {
	if knowledge.IsSchemaObject(value.Address.ObjectID) {
		return CompiledDoc{}, false
	}
	bound := boundSpec(repo, value, spec)
	return CompiledDoc{
		ObjectID: value.Address.ObjectID,
		Text:     documentText(value, bound),
		Fields:   indexedFields(value, bound),
	}, true
}

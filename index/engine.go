package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

// ProjectionCell is one typed, provider-neutral realization of an AccessField.
// Field is the complete FieldRef key; providers must not index a bare JSON path.
// Value is retained as the canonical scalar representation for reference
// providers. Scale providers should use the matching typed slot.
type ProjectionCell struct {
	Field        string   `json:"field"`
	Value        string   `json:"value"`
	StringValue  *string  `json:"stringValue,omitempty"`
	TextValue    string   `json:"textValue,omitempty"`
	LongValue    *int64   `json:"longValue,omitempty"`
	DoubleValue  *float64 `json:"doubleValue,omitempty"`
	BooleanValue *bool    `json:"booleanValue,omitempty"`
	DateValue    string   `json:"dateValue,omitempty"`
}

type ProjectionRelationEndpoint struct {
	Role      string             `json:"role"`
	ObjectRef knowledge.ObjectID `json:"objectRef"`
}

type ProjectionRelation struct {
	Type      string                       `json:"type"`
	Direction knowledge.RelationDirection  `json:"direction"`
	Endpoints []ProjectionRelationEndpoint `json:"endpoints"`
}

// CompiledDoc is one complete knowledge object extracted from AccessHints.
// Schema objects never appear. Aspect/member operations invalidate this object;
// they are deliberately not physical document boundaries.
type CompiledDoc struct {
	ObjectID       knowledge.ObjectID    `json:"objectId"`
	Kind           knowledge.AddressKind `json:"kind"`
	Text           string                `json:"text"`
	EligibleFields []string              `json:"eligibleFields"`
	Cells          []ProjectionCell      `json:"cells"`
	Relation       *ProjectionRelation   `json:"relation,omitempty"`
	ObjectDigest   kernel.Digest         `json:"objectDigest"`

	// Fields is a compatibility view used by the SQLite reference provider.
	// It carries the same complete FieldRef key and canonical scalar value as Cells.
	Fields [][2]string `json:"-"`
}

// Meta is projection basis stored by an Engine.
type Meta struct {
	Basis            kernel.CommitID
	AccessDigest     kernel.Digest
	PhysicalDigest   kernel.Digest
	ProviderRevision string
	Generation       string
	State            string
	Coverage         float64
	Mode             string
	Cause            string
}

const (
	ProjectionStateBuilding = "BUILDING"
	ProjectionStateReady    = "READY"
	ProjectionStateUpdating = "UPDATING"
	ProjectionStateFailed   = "FAILED"
	ProjectionStateRetired  = "RETIRED"
)

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
	Repository kernel.RepositoryID      `json:"repository"`
	ObjectID   knowledge.ObjectID       `json:"objectId"`
	Basis      kernel.CommitID          `json:"basis"`
	Evidence   []retrieval.LaneEvidence `json:"evidence"`
}

type CandidatePage struct {
	Candidates   []CandidateRef `json:"candidates"`
	Continuation string         `json:"continuation,omitempty"`
	Exhausted    bool           `json:"exhausted"`
}

type RetrieveRequest struct {
	Search       retrieval.SearchRequest `json:"search"`
	Spec         retrieval.AccessSpec    `json:"spec"`
	Continuation string                  `json:"continuation,omitempty"`
}

// Retriever locates typed candidates. It never returns knowledge payload.
type Retriever interface {
	Probe(retrieval.SearchClause, retrieval.AccessSpec) Capability
	Retrieve(RetrieveRequest) (CandidatePage, error)
}

type RelationRetrieveRequest struct {
	Query        reader.RelationQuery `json:"query"`
	Limit        int                  `json:"limit,omitempty"`
	Continuation string               `json:"continuation,omitempty"`
}

type RelationCandidatePage struct {
	Candidates   []CandidateRef `json:"candidates"`
	Continuation string         `json:"continuation,omitempty"`
	Exhausted    bool           `json:"exhausted"`
}

// RelationRetriever is the optional reserved lane for one-hop relation
// location. Relation attributes still use normal declarative AccessSpec cells.
type RelationRetriever interface {
	RetrieveRelations(RelationRetrieveRequest) (RelationCandidatePage, error)
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

func compileValue(repo knowledge.Repository, value knowledge.KnowledgeValue, spec retrieval.AccessSpec) (CompiledDoc, bool, error) {
	if knowledge.IsSchemaObject(value.Address.ObjectID) {
		return CompiledDoc{}, false, nil
	}
	doc, err := compileProjectionDocument(repo, value, spec)
	if err != nil {
		return CompiledDoc{}, false, err
	}
	return doc, true, nil
}

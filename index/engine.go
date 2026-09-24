package index

import (
	"context"
	"kc/kernel"
	"kc/knowledge"
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
	Role      string                 `json:"role"`
	ObjectRef knowledge.KnowledgeRef `json:"objectRef"`
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

	// Fields is a compatibility view used by provider conformance and adapters
	// that still consume canonical scalar pairs. New providers should use Cells.
	Fields [][2]string `json:"-"`
}

// Meta is projection basis stored by an Engine.
type Meta struct {
	Basis             kernel.CommitID
	AccessDigest      kernel.Digest
	ObservationDigest kernel.Digest
	PhysicalDigest    kernel.Digest
	ProviderRevision  string
	Generation        string
	Revision          string
	State             string
	Coverage          float64
	Mode              string
	Cause             string
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

// ExpressionProber proves that a Retriever implements the composition of
// already-supported leaves. Leaf Probe alone cannot establish All/Any
// semantics. Retrievers that do not implement this port remain valid for the
// legacy implicit-All request, but explicit SearchExpr requests fail closed.
type ExpressionProber interface {
	ProbeExpression(retrieval.SearchExpr, retrieval.AccessSpec) Capability
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

// ContextRetriever is the optional cancellable execution port. Legacy
// providers remain valid; the executor checks cancellation between their calls.
type ContextRetriever interface {
	RetrieveContext(context.Context, RetrieveRequest) (CandidatePage, error)
}

// SemanticWindowRetriever is the optional vector-window port (docs/reviewed/retrieval.md
// §8.1). A provider that cannot serve a derived k-NN window simply does not
// implement it; callers must then fail closed instead of downgrading to
// lexical recall. A window is one bounded top-K request: no continuation, and
// every candidate carries an approximate-guarantee lane evidence.
type SemanticWindowRetriever interface {
	SemanticWindowContext(context.Context, RetrieveRequest, []float32) (CandidatePage, error)
}

// ResidualMatchVerifier supplies the same analyzed term/phrase semantics as
// the provider's MATCH implementation. Without it, a Superset plan containing
// MATCH cannot be verified and must fail before candidate retrieval.
type ResidualMatchVerifier interface {
	VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error)
}

// ContextMetaLoader lets readiness and fixed-basis selection use the caller deadline.
type ContextMetaLoader interface {
	LoadMetaContext(context.Context) (Meta, error)
}

func loadMetaContext(ctx context.Context, engine Engine) (Meta, error) {
	if err := ctx.Err(); err != nil {
		return Meta{}, err
	}
	if loader, ok := engine.(ContextMetaLoader); ok {
		return loader.LoadMetaContext(ctx)
	}
	meta, err := engine.LoadMeta()
	if ctx.Err() != nil {
		return Meta{}, ctx.Err()
	}
	return meta, err
}

// ProjectionMaintainer owns only discardable physical projection state.
type ProjectionMaintainer interface {
	LoadMeta() (Meta, error)
	Rebuild(docs []CompiledDoc, meta Meta) error
	Apply(upserts []CompiledDoc, deletes []knowledge.ObjectID, meta Meta) error
	Count() (int, error)
}

// RebuildSession lets scale providers build a generation in bounded batches.
// Commit atomically publishes it; Abort leaves the previous READY generation.
type RebuildSession interface {
	Append([]CompiledDoc) error
	Commit() error
	Abort(error) error
}

type StreamingProjectionMaintainer interface {
	BeginRebuild(Meta) (RebuildSession, error)
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

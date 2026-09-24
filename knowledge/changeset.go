package knowledge

import "kc/kernel"

type OpKind string

const (
	OpPut    OpKind = "PUT"
	OpRemove OpKind = "REMOVE"
)

type PreconditionType string

const (
	IfAbsent       PreconditionType = "IF_ABSENT"
	IfObjectEquals PreconditionType = "IF_OBJECT_EQUALS"
	IfDigestEquals PreconditionType = "IF_DIGEST_EQUALS"
)

type Precondition struct {
	Type   PreconditionType `json:"type"`
	Digest kernel.Digest    `json:"digest,omitempty"`
}

type Operation struct {
	Op           OpKind        `json:"op"`
	Address      Address       `json:"address"`
	Value        any           `json:"value,omitempty"`
	PathHint     string        `json:"pathHint,omitempty"`
	SchemaRef    string        `json:"schemaRef,omitempty"`
	ValueSource  *ValueSource  `json:"valueSource,omitempty"`
	Precondition *Precondition `json:"precondition,omitempty"`
	Reason       string        `json:"reason,omitempty"`
	Replacement  *KnowledgeRef `json:"replacement,omitempty"`
}

type ChangeSet struct {
	TargetRepository     kernel.RepositoryID `json:"targetRepository"`
	TargetRef            string              `json:"targetRef"`
	BaseCommit           kernel.CommitID     `json:"baseCommit"`
	ExpectedTargetCommit kernel.CommitID     `json:"expectedTargetCommit"`
	Operations           []Operation         `json:"operations"`
	Message              string              `json:"message,omitempty"`
	Author               string              `json:"author,omitempty"`
	RequestID            string              `json:"requestId,omitempty"`
	RuleID               string              `json:"ruleId,omitempty"`
	Provenance           *ProvenanceEnvelope `json:"provenance,omitempty"`
	// BulkIngest requests the target medium's bulk data plane for this
	// ChangeSet when the medium provides it. It changes object transport
	// only — CAS, commit identity and content are identical — and is covered
	// by the command digest (omitempty: false stays out of the canonical
	// JSON, so existing digests are unchanged), so replaying one command_id
	// with a flipped mode deterministically conflicts instead of silently
	// switching paths.
	BulkIngest bool `json:"bulkIngest,omitempty"`
}

type CommitChangeSet = ChangeSet

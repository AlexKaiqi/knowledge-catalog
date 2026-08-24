package repository

import "kc/kernel"

// COMMIT: PUT / REMOVE on a Snapshot. PROPOSAL uses the same ChangeSet on a candidate Ref.

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
	Op           OpKind               `json:"op"`
	Address      kernel.Address       `json:"address"`
	Value        any                  `json:"value,omitempty"`
	PathHint     string               `json:"pathHint,omitempty"`
	SchemaRef    string               `json:"schemaRef,omitempty"`
	Precondition *Precondition        `json:"precondition,omitempty"`
	Reason       string               `json:"reason,omitempty"`
	Replacement  *kernel.KnowledgeRef `json:"replacement,omitempty"`
}

type CommitChangeSet struct {
	TargetRepository     kernel.RepositoryID        `json:"targetRepository"`
	TargetRef            string                     `json:"targetRef"`
	BaseCommit           kernel.CommitID            `json:"baseCommit"`
	ExpectedTargetCommit kernel.CommitID            `json:"expectedTargetCommit"`
	Operations           []Operation                `json:"operations"`
	Message              string                     `json:"message,omitempty"`
	Author               string                     `json:"author,omitempty"`
	RequestID            string                     `json:"requestId,omitempty"`
	RuleID               string                     `json:"ruleId,omitempty"`
	Provenance           *kernel.ProvenanceEnvelope `json:"provenance,omitempty"`
}

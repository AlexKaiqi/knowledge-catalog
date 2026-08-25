package connector

import (
	"kc/kernel"
	"kc/knowledge"
)

// Mode selects whether absence in Desired may REMOVE.
type Mode string

const (
	// ModePatch PUTs Desired units only. Extra Observed addresses stay.
	ModePatch Mode = "patch"
	// ModeReconcile set-diffs Desired against Observed ∩ Scope, including REMOVE.
	ModeReconcile Mode = "reconcile"
)

// SourceKey is an identifier in the external system. It is not object_id.
type SourceKey string

// Signal is a change notification. Keys are not new Aspect values.
type Signal struct {
	Keys []SourceKey `json:"keys"`
	At   string      `json:"at,omitempty"`
}

// Unit is one translated maintenance unit. The connector mints object_id, not the kit.
type Unit struct {
	Address   knowledge.Address `json:"address"`
	Value     any               `json:"value"`
	SchemaRef string            `json:"schemaRef,omitempty"`
	PathHint  string            `json:"pathHint,omitempty"`
	SourceKey SourceKey         `json:"sourceKey,omitempty"`
}

// Observed is a catalog digest for one Address. The caller READs; the kit does not.
type Observed struct {
	Address knowledge.Address `json:"address"`
	Digest  kernel.Digest     `json:"digest"`
}

// Scope is the Addresses one connector may write. Split by change source, not by entity.
type Scope struct {
	Aspects      []string `json:"aspects,omitempty"`
	AllowEntity  bool     `json:"allowEntity,omitempty"`
	ObjectPrefix string   `json:"objectPrefix,omitempty"`
}

// Plan is the input to Preview. Pure data; no source client.
type Plan struct {
	ConnectorID      string              `json:"connectorId"`
	Mode             Mode                `json:"mode"`
	Scope            Scope               `json:"scope"`
	TargetRepository kernel.RepositoryID `json:"targetRepository"`
	TargetRef        string              `json:"targetRef,omitempty"`
	BaseCommit       kernel.CommitID     `json:"baseCommit,omitempty"`
	Desired          []Unit              `json:"desired"`
	Observed         []Observed          `json:"observed"`
	SourceRefs       []string            `json:"sourceRefs"`
	ProducedAt       string              `json:"producedAt,omitempty"`
	ActorRef         string              `json:"actorRef,omitempty"`
	Message          string              `json:"message,omitempty"`
}

// Checkpoint is connector-owned sync cursor. The kit never stores it.
type Checkpoint struct {
	ConnectorID         string          `json:"connectorId"`
	Cursor              string          `json:"cursor,omitempty"`
	LastCatalogCommit   kernel.CommitID `json:"lastCatalogCommit,omitempty"`
	LastFullReconcileAt string          `json:"lastFullReconcileAt,omitempty"`
	ProducedAt          string          `json:"producedAt,omitempty"`
}

// Summary counts Preview outcomes.
type Summary struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Removed   int `json:"removed"`
	Unchanged int `json:"unchanged"`
	Ignored   int `json:"ignored"`
}

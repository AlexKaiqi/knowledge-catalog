package client

import (
	"encoding/json"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
)

type ObjectDatasetIdentity struct {
	Dataset    string           `json:"dataset,omitempty"`
	Revision   int              `json:"revision,omitempty"`
	Pin        json.RawMessage  `json:"pin,omitempty"`
	Definition *dataset.Dataset `json:"definition,omitempty"`
}

type ObjectRepoPin struct {
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

type ObjectScope struct {
	Dataset    string           `json:"dataset,omitempty"`
	Revision   int              `json:"revision,omitempty"`
	Pin        json.RawMessage  `json:"pin,omitempty"`
	Definition *dataset.Dataset `json:"definition,omitempty"`
	Repository string           `json:"repository,omitempty"`
	Commit     string           `json:"commit,omitempty"`
	Ref        string           `json:"ref,omitempty"`
}

type ReadRequest struct {
	ObjectDatasetIdentity
	Reference  string `json:"reference"`
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

type ObjectRequest struct {
	ObjectScope
	Object       string `json:"object"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type ResolveRequest struct {
	ObjectScope
	Object string `json:"object"`
	Aspect string `json:"aspect,omitempty"`
	Member string `json:"member,omitempty"`
}

type SchemaRequest struct {
	ObjectScope
	Object string `json:"object,omitempty"`
}

type SchemaPageRequest struct {
	ObjectRepoPin
	Dataset      string `json:"dataset,omitempty"`
	Revision     int    `json:"revision,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type CommitRequest struct {
	CommandID string                `json:"commandId"`
	ChangeSet objectmodel.ChangeSet `json:"changeSet"`
}

// MaintenanceRequest keeps public content selection separate from server-owned
// maintenance-unit resolution. The SDK never infers write scope from depth.
type MaintenanceRequest struct {
	Reference   string                          `json:"reference"`
	Repository  string                          `json:"repository"`
	CommandID   string                          `json:"commandId"`
	Ref         string                          `json:"ref,omitempty"`
	Base        string                          `json:"base,omitempty"`
	Expected    string                          `json:"expected,omitempty"`
	Value       any                             `json:"value"`
	Kind        string                          `json:"kind,omitempty"`
	SchemaRef   string                          `json:"schemaRef,omitempty"`
	PathHint    string                          `json:"pathHint,omitempty"`
	IfAbsent    bool                            `json:"ifAbsent,omitempty"`
	IfDigest    string                          `json:"ifDigest,omitempty"`
	Message     string                          `json:"message,omitempty"`
	ValueSource *objectmodel.ValueSource        `json:"valueSource,omitempty"`
	Provenance  *objectmodel.ProvenanceEnvelope `json:"provenance,omitempty"`
}

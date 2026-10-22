package object

import "kc/kernel"

type ResolutionStatus string

const (
	StatusResolved   ResolutionStatus = "RESOLVED"
	StatusRemoved    ResolutionStatus = "REMOVED"
	StatusUnresolved ResolutionStatus = "UNRESOLVED"
	StatusForbidden  ResolutionStatus = "FORBIDDEN"
)

// Resolution identifies a fixed maintenance target. Digest is its conditional
// write/remove basis (MaintenanceDigest or UnitSetMaintenanceDigest), distinct
// from the content digest returned by READ for a binding declaration.
type Resolution struct {
	Repository        kernel.RepositoryID `json:"repository"`
	Commit            kernel.CommitID     `json:"commit"`
	ObjectID          ObjectID            `json:"objectId"`
	Address           Address             `json:"address"`
	PathHint          string              `json:"pathHint"`
	Digest            kernel.Digest       `json:"digest,omitempty"`
	DeclarationDigest kernel.Digest       `json:"declarationDigest,omitempty"`
	SchemaRef         string              `json:"schemaRef,omitempty"`
	ValueSource       *ValueSource        `json:"valueSource,omitempty"`
	Status            ResolutionStatus    `json:"status"`
}

type ObjectValue struct {
	Basis        string              `json:"basis,omitempty"`
	State        *ReadState          `json:"state,omitempty"`
	Reference    string              `json:"reference,omitempty"`
	Locator      string              `json:"locator,omitempty"`
	Digest       kernel.Digest       `json:"digest,omitempty"`
	ObjectRef    ObjectRef           `json:"objectRef"`
	Repository   kernel.RepositoryID `json:"repository"`
	Commit       kernel.CommitID     `json:"commit"`
	Address      Address             `json:"address"`
	Value        any                 `json:"value"`
	Provenance   *ProvenanceEnvelope `json:"provenance,omitempty"`
	Units        []Address           `json:"units,omitempty"`
	Declarations []UnitDeclaration   `json:"declarations,omitempty"`
	// Sources is the version vector when the scope resolved the object across
	// Repositories; Withheld names holders the caller may not read.
	Sources  []SourceBasis         `json:"sources,omitempty"`
	Withheld []kernel.RepositoryID `json:"withheld,omitempty"`
}

type ProvenanceTrace struct {
	Repository kernel.RepositoryID  `json:"repository"`
	Commit     kernel.CommitID      `json:"commit"`
	ObjectID   ObjectID             `json:"objectId"`
	Chain      []ProvenanceEnvelope `json:"chain"`
}

type FederatedValue struct {
	Basis        string                `json:"basis,omitempty"`
	State        *ReadState            `json:"state,omitempty"`
	Reference    string                `json:"reference,omitempty"`
	Locator      string                `json:"locator,omitempty"`
	Digest       kernel.Digest         `json:"digest,omitempty"`
	ObjectRef    ObjectRef             `json:"objectRef"`
	Repository   kernel.RepositoryID   `json:"repository"`
	Commit       kernel.CommitID       `json:"commit"`
	ObjectID     ObjectID              `json:"objectId"`
	Address      Address               `json:"address"`
	Value        any                   `json:"value"`
	Provenance   *ProvenanceEnvelope   `json:"provenance,omitempty"`
	Units        []Address             `json:"units,omitempty"`
	Declarations []UnitDeclaration     `json:"declarations,omitempty"`
	Sources      []SourceBasis         `json:"sources,omitempty"`
	Withheld     []kernel.RepositoryID `json:"withheld,omitempty"`
}

// ObservedValue retains a fixed declaration and independently observed values.
type ObservedValue struct {
	FederatedValue
	Observations []UnitObservation `json:"observations,omitempty"`
	Failures     []UnitReadFailure `json:"failures,omitempty"`
}

// UnitReadFailure identifies an external unit whose value could not be adopted.
// It is not a successful observation and carries no ObservationBasis.
type UnitReadFailure struct {
	Address Address          `json:"address"`
	Code    kernel.ErrorCode `json:"code"`
	Message string           `json:"message"`
}

package object

import "kc/kernel"

type ResolutionStatus string

const (
	StatusResolved   ResolutionStatus = "RESOLVED"
	StatusRemoved    ResolutionStatus = "REMOVED"
	StatusUnresolved ResolutionStatus = "UNRESOLVED"
	StatusForbidden  ResolutionStatus = "FORBIDDEN"
)

type AspectSelector struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

func SelectAspects(value any, units []Address, selector *AspectSelector) any {
	if selector == nil {
		return value
	}
	hasAspect := false
	for _, unit := range units {
		if unit.AspectName != "" {
			hasAspect = true
			break
		}
	}
	if !hasAspect {
		return value
	}
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	exclude := map[string]struct{}{}
	for _, name := range selector.Exclude {
		exclude[name] = struct{}{}
	}
	out := map[string]any{}
	for name, item := range object {
		if len(selector.Include) > 0 {
			found := false
			for _, include := range selector.Include {
				if include == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if _, skip := exclude[name]; skip {
			continue
		}
		out[name] = item
	}
	return out
}

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

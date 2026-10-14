package interfaces

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
)

type InterfaceRequest struct {
	Repository     kernel.RepositoryID  `json:"repository"`
	Object         objectmodel.ObjectID `json:"object"`
	Implementation string               `json:"implementation"`
	InterfaceRef   string               `json:"interfaceRef,omitempty"`
	Operation      string               `json:"operation"`
	Input          any                  `json:"input,omitempty"`
}

// InterfaceCall contains only resolved coordinates and a trusted caller
// context. Handlers cannot replace the operation contract with current HEAD.
type InterfaceCall struct {
	Implementation ResolvedInterface
	Operation      string
	Input          any
	Request        objectmodel.RequestContext
	// Declarations is a scope-preserving reader for materials required by an
	// extension. It never exposes an unscoped repository lookup.
	Declarations DeclarationReader
}

type InterfaceResult struct {
	Value any `json:"value"`
	Basis any `json:"basis,omitempty"`
}

type InterfaceExecutionBasis struct {
	DeclarationCommit                   kernel.CommitID                `json:"declarationCommit"`
	DeclarationDigest                   kernel.Digest                  `json:"declarationDigest"`
	Contract                            objectmodel.PinnedKnowledgeRef `json:"contract"`
	ContractDigest                      kernel.Digest                  `json:"contractDigest"`
	ImplementationSchema                objectmodel.PinnedKnowledgeRef `json:"implementationSchema"`
	ImplementationSchemaDigest          kernel.Digest                  `json:"implementationSchemaDigest"`
	ImplementationSchemaStructureDigest kernel.Digest                  `json:"implementationSchemaStructureDigest"`
	HandlerRevision                     string                         `json:"handlerRevision"`
}

type InterfaceCallResult struct {
	Value       any                     `json:"value"`
	Observation any                     `json:"observation,omitempty"`
	Basis       InterfaceExecutionBasis `json:"basis"`
}

// InterfaceAuthorizer applies operation-specific policy after resolution and
// before handler execution. A write operation always requires this explicit port.
type InterfaceAuthorizer func(context.Context, InterfaceCall) error

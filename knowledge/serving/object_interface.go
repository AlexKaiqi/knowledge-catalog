package serving

import (
	"context"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

type InterfaceRequest struct {
	Repository     kernel.RepositoryID `json:"repository"`
	Object         knowledge.ObjectID  `json:"object"`
	Implementation string              `json:"implementation"`
	InterfaceRef   string              `json:"interfaceRef,omitempty"`
	Operation      string              `json:"operation"`
	Input          any                 `json:"input,omitempty"`
}

// InterfaceCall contains only resolved coordinates and a trusted caller
// context. Handlers cannot replace the operation contract with current HEAD.
type InterfaceCall struct {
	Implementation reader.ResolvedInterface
	Operation      string
	Input          any
	Request        RequestContext
	// Declarations is a scope-preserving reader for materials required by an
	// extension. It never exposes an unscoped repository lookup.
	Declarations *reader.Serving
}

type InterfaceResult struct {
	Value any `json:"value"`
	Basis any `json:"basis,omitempty"`
}

type InterfaceExecutionBasis struct {
	DeclarationCommit                   kernel.CommitID              `json:"declarationCommit"`
	DeclarationDigest                   kernel.Digest                `json:"declarationDigest"`
	Contract                            knowledge.PinnedKnowledgeRef `json:"contract"`
	ContractDigest                      kernel.Digest                `json:"contractDigest"`
	ImplementationSchema                knowledge.PinnedKnowledgeRef `json:"implementationSchema"`
	ImplementationSchemaDigest          kernel.Digest                `json:"implementationSchemaDigest"`
	ImplementationSchemaStructureDigest kernel.Digest                `json:"implementationSchemaStructureDigest"`
	HandlerRevision                     string                       `json:"handlerRevision"`
}

type InterfaceCallResult struct {
	Value       any                     `json:"value"`
	Observation any                     `json:"observation,omitempty"`
	Basis       InterfaceExecutionBasis `json:"basis"`
}

// InterfaceAuthorizer applies operation-specific policy after resolution and
// before handler execution. A write operation always requires this explicit port.
type InterfaceAuthorizer func(context.Context, InterfaceCall) error

type InterfaceService struct {
	declarations *reader.Serving
	registry     *InterfaceRegistry
	request      RequestContext
	authorize    InterfaceAuthorizer
}

func OpenInterfaces(declarations *reader.Serving, registry *InterfaceRegistry, request RequestContext) *InterfaceService {
	return &InterfaceService{declarations: declarations, registry: registry, request: request}
}

func (s *InterfaceService) SetAuthorizer(authorizer InterfaceAuthorizer) { s.authorize = authorizer }

func (s *InterfaceService) Call(ctx context.Context, request InterfaceRequest) (InterfaceCallResult, error) {
	if s.declarations == nil || request.Repository == "" || request.Object == "" || request.Implementation == "" || request.Operation == "" {
		return InterfaceCallResult{}, kernel.Fail(kernel.ErrUsageInvalid, "interface call requires pinned declarations, repository, object, implementation and operation")
	}
	if s.request.Identity.Principal == "" {
		return InterfaceCallResult{}, kernel.Fail(kernel.ErrUsageInvalid, "interface call requires explicit trusted principal")
	}
	implementation, err := s.declarations.ResolveObjectInterfaceAt(request.Repository, request.Object, request.Implementation)
	if err != nil {
		return InterfaceCallResult{}, err
	}
	if request.InterfaceRef != "" && request.InterfaceRef != string(implementation.Contract.Object) && request.InterfaceRef != knowledge.FormatPinnedRef(implementation.Contract.Repository, implementation.Contract.Commit, implementation.Contract.Object) {
		return InterfaceCallResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "selected implementation does not implement requested interface %s", request.InterfaceRef)
	}
	operation, exists := implementation.Definition.Operations[request.Operation]
	if !exists {
		return InterfaceCallResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "interface %s does not declare operation %s", implementation.Contract.Object, request.Operation)
	}
	registration, err := s.registry.resolve(implementation)
	if err != nil {
		return InterfaceCallResult{}, err
	}
	if schema, ok := implementation.InputSchemas[request.Operation]; ok {
		if err := validateInterfaceValue(request.Input, schema); err != nil {
			return InterfaceCallResult{}, err
		}
	}
	call := InterfaceCall{Implementation: implementation, Operation: request.Operation, Input: request.Input, Request: s.request, Declarations: s.declarations}
	if operation.Effect == "write" && s.authorize == nil {
		return InterfaceCallResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "interface operation %s requires explicit execution authorization", request.Operation)
	}
	if s.authorize != nil {
		authorizationCall, err := isolateInterfaceCall(call)
		if err != nil {
			return InterfaceCallResult{}, err
		}
		if err := s.authorize(ctx, authorizationCall); err != nil {
			return InterfaceCallResult{}, err
		}
	}
	validationCall, err := isolateInterfaceCall(call)
	if err != nil {
		return InterfaceCallResult{}, err
	}
	if err := registration.Handler.ValidateImplementation(ctx, validationCall.Implementation); err != nil {
		return InterfaceCallResult{}, err
	}
	executionCall, err := isolateInterfaceCall(call)
	if err != nil {
		return InterfaceCallResult{}, err
	}
	result, err := registration.Handler.Execute(ctx, executionCall)
	if err != nil {
		return InterfaceCallResult{}, err
	}
	if schema, ok := implementation.ResultSchemas[request.Operation]; ok {
		if err := validateInterfaceValue(result.Value, schema); err != nil {
			return InterfaceCallResult{}, err
		}
	}
	return InterfaceCallResult{Value: result.Value, Observation: result.Basis, Basis: InterfaceExecutionBasis{DeclarationCommit: implementation.Commit, DeclarationDigest: implementation.DeclarationDigest, Contract: implementation.Contract, ContractDigest: implementation.ContractDigest, ImplementationSchema: implementation.ImplementationSchema, ImplementationSchemaDigest: implementation.ImplementationSchemaDigest, ImplementationSchemaStructureDigest: implementation.ImplementationSchemaStructureDigest, HandlerRevision: registration.HandlerRevision}}, nil
}

func validateInterfaceValue(value any, schema knowledge.SchemaDefinition) error {
	address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: schema.ObjectID}
	if schema.Aspect != "" {
		address.Kind = knowledge.KindAspect
		address.AspectName = schema.Aspect
	}
	if schema.Pattern == "keyed_collection" {
		address.Kind = knowledge.KindMember
		address.MemberKey = "value"
	}
	return knowledge.ValidateSchemaInstance(address, value, schema)
}

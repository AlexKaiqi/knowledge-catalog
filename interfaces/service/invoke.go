package service

import (
	"context"
	"kc/interfaces"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type Service struct {
	declarations *reader.Serving
	registry     *interfaces.InterfaceRegistry
	request      objectmodel.RequestContext
	authorize    interfaces.InterfaceAuthorizer
}

func Open(declarations *reader.Serving, registry *interfaces.InterfaceRegistry, request objectmodel.RequestContext) *Service {
	return &Service{declarations: declarations, registry: registry, request: request}
}

func (s *Service) SetAuthorizer(authorizer interfaces.InterfaceAuthorizer) { s.authorize = authorizer }

func (s *Service) Call(ctx context.Context, request interfaces.InterfaceRequest) (interfaces.InterfaceCallResult, error) {
	if s.declarations == nil || request.Repository == "" || request.Object == "" || request.Implementation == "" || request.Operation == "" {
		return interfaces.InterfaceCallResult{}, kernel.Fail(kernel.ErrUsageInvalid, "interface call requires pinned declarations, repository, object, implementation and operation")
	}
	if s.request.Identity.Principal == "" {
		return interfaces.InterfaceCallResult{}, kernel.Fail(kernel.ErrUsageInvalid, "interface call requires explicit trusted principal")
	}
	implementation, err := interfaces.ResolveObjectInterfaceAt(s.declarations, request.Repository, request.Object, request.Implementation)
	if err != nil {
		return interfaces.InterfaceCallResult{}, err
	}
	if request.InterfaceRef != "" && request.InterfaceRef != string(implementation.Contract.Object) && request.InterfaceRef != objectmodel.FormatPinnedRef(implementation.Contract.Repository, implementation.Contract.Commit, implementation.Contract.Object) {
		return interfaces.InterfaceCallResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "selected implementation does not implement requested interface %s", request.InterfaceRef)
	}
	operation, exists := implementation.Definition.Operations[request.Operation]
	if !exists {
		return interfaces.InterfaceCallResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "interface %s does not declare operation %s", implementation.Contract.Object, request.Operation)
	}
	registration, err := s.registry.Resolve(implementation)
	if err != nil {
		return interfaces.InterfaceCallResult{}, err
	}
	if schema, ok := implementation.InputSchemas[request.Operation]; ok {
		if err := validateInterfaceValue(request.Input, schema); err != nil {
			return interfaces.InterfaceCallResult{}, err
		}
	}
	call := interfaces.InterfaceCall{Implementation: implementation, Operation: request.Operation, Input: request.Input, Request: s.request, Declarations: fixedDeclarations{scope: s.declarations}}
	if operation.Effect == "write" && s.authorize == nil {
		return interfaces.InterfaceCallResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "interface operation %s requires explicit execution authorization", request.Operation)
	}
	if s.authorize != nil {
		authorizationCall, err := isolateInterfaceCall(call)
		if err != nil {
			return interfaces.InterfaceCallResult{}, err
		}
		if err := s.authorize(ctx, authorizationCall); err != nil {
			return interfaces.InterfaceCallResult{}, err
		}
	}
	validationCall, err := isolateInterfaceCall(call)
	if err != nil {
		return interfaces.InterfaceCallResult{}, err
	}
	if err := registration.Handler.ValidateImplementation(ctx, validationCall.Implementation); err != nil {
		return interfaces.InterfaceCallResult{}, err
	}
	executionCall, err := isolateInterfaceCall(call)
	if err != nil {
		return interfaces.InterfaceCallResult{}, err
	}
	result, err := registration.Handler.Execute(ctx, executionCall)
	if err != nil {
		return interfaces.InterfaceCallResult{}, err
	}
	if schema, ok := implementation.ResultSchemas[request.Operation]; ok {
		if err := validateInterfaceValue(result.Value, schema); err != nil {
			return interfaces.InterfaceCallResult{}, err
		}
	}
	return interfaces.InterfaceCallResult{Value: result.Value, Observation: result.Basis, Basis: interfaces.InterfaceExecutionBasis{DeclarationCommit: implementation.Commit, DeclarationDigest: implementation.DeclarationDigest, Contract: implementation.Contract, ContractDigest: implementation.ContractDigest, ImplementationSchema: implementation.ImplementationSchema, ImplementationSchemaDigest: implementation.ImplementationSchemaDigest, ImplementationSchemaStructureDigest: implementation.ImplementationSchemaStructureDigest, HandlerRevision: registration.HandlerRevision}}, nil
}

func validateInterfaceValue(value any, schema objectmodel.SchemaDefinition) error {
	address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schema.ObjectID}
	if schema.Aspect != "" {
		address.Kind = objectmodel.KindAspect
		address.AspectName = schema.Aspect
	}
	if schema.Pattern == "keyed_collection" {
		address.Kind = objectmodel.KindMember
		address.MemberKey = "value"
	}
	return objectmodel.ValidateSchemaInstance(address, value, schema)
}

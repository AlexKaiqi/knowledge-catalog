package serving

import (
	"context"
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

// LiveAccessHandler implements the published LiveAccess contract through the
// injected resource-access/v1 adapter. Source clients remain in that runtime.
type LiveAccessHandler struct{ accessor ResourceOperationAccessor }

func NewLiveAccessHandler(accessor ResourceOperationAccessor) *LiveAccessHandler {
	return &LiveAccessHandler{accessor: accessor}
}

func (h *LiveAccessHandler) ValidateImplementation(_ context.Context, implementation reader.ResolvedInterface) error {
	value := implementation.Implementation
	for _, field := range []string{"runtime", "protocol", "origin"} {
		if text, ok := value[field].(string); !ok || strings.TrimSpace(text) == "" {
			return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "LiveAccess requires %s", field)
		}
	}
	if value["protocol"] != knowledge.ProtocolResourceAccessV1 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess HTTP handler does not support protocol %v", value["protocol"])
	}
	if _, err := knowledge.ResourceAccessEndpoint(value["origin"].(string)); err != nil {
		return err
	}
	operations, ok := value["operations"].(map[string]any)
	if !ok || len(operations) == 0 {
		return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "LiveAccess requires operations")
	}
	for operation, raw := range operations {
		if _, err := liveAccessCall(operations, operation); err != nil {
			return err
		}
		if _, ok := raw.(map[string]any); !ok {
			return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "LiveAccess operation must be an object")
		}
	}
	return nil
}

func (h *LiveAccessHandler) Execute(ctx context.Context, call InterfaceCall) (InterfaceResult, error) {
	if h == nil || h.accessor == nil {
		return InterfaceResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess runtime adapter is unavailable")
	}
	if err := h.ValidateImplementation(ctx, call.Implementation); err != nil {
		return InterfaceResult{}, err
	}
	value := call.Implementation.Implementation
	operationCall, err := liveAccessCall(value["operations"].(map[string]any), call.Operation)
	if err != nil {
		return InterfaceResult{}, err
	}
	address := call.Implementation.Address
	result, err := h.accessor.AccessResource(ctx, ResourceOperationRequest{
		Descriptor:     ResourceDescriptorCoordinate{ObjectID: address.ObjectID, Repository: call.Implementation.Repository, Commit: call.Implementation.Commit},
		Implementation: &address, DeclarationDigest: call.Implementation.DeclarationDigest,
		Runtime: value["runtime"].(string), Protocol: value["protocol"].(string), Origin: value["origin"].(string),
		Operation: call.Operation, Call: operationCall, Input: call.Input,
		Identity: StateRuntimeIdentity{Principal: call.Request.Identity.Principal, OnBehalfOf: call.Request.Identity.OnBehalfOf, RequestID: call.Request.RequestID, TraceID: call.Request.Trace.TraceID, SpanID: call.Request.Trace.SpanID, ParentSpanID: call.Request.Trace.ParentSpanID},
	})
	if err != nil {
		return InterfaceResult{}, err
	}
	if call.Operation != "read" {
		return InterfaceResult{Value: result}, nil
	}
	return liveAccessObservation(result)
}

func liveAccessCall(operations map[string]any, operation string) (string, error) {
	value, ok := operations[operation].(map[string]any)
	if !ok {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess does not implement operation %s", operation)
	}
	call, ok := value["call"].(string)
	if !ok || strings.TrimSpace(call) == "" {
		return "", kernel.Fail(kernel.ErrSchemaInstanceInvalid, "LiveAccess operation %s requires call", operation)
	}
	return strings.TrimSpace(call), nil
}

func liveAccessObservation(value any) (InterfaceResult, error) {
	envelope, ok := value.(map[string]any)
	if !ok {
		return InterfaceResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess read requires value and observation basis")
	}
	observed, exists := envelope["value"]
	if !exists {
		return InterfaceResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess read requires value (use null for observed null)")
	}
	raw, err := json.Marshal(envelope["basis"])
	if err != nil {
		return InterfaceResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess observation basis is invalid: %v", err)
	}
	var basis knowledge.ObservationBasis
	if err := json.Unmarshal(raw, &basis); err != nil {
		return InterfaceResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess observation basis is invalid: %v", err)
	}
	if err := knowledge.ValidateObservationBasis(basis); err != nil {
		return InterfaceResult{}, err
	}
	return InterfaceResult{Value: observed, Basis: basis}, nil
}

// RegisterLiveAccess installs this handler for an explicitly resolved release.
// Matching the trusted contract digest and schema structure prevents a local contract edit from
// turning the standard write operation into an implicitly authorized read.
func RegisterLiveAccess(registry *InterfaceRegistry, implementation reader.ResolvedInterface, accessor ResourceOperationAccessor) error {
	contract, schema, err := liveAccessPublicationBasis()
	if err != nil {
		return err
	}
	if implementation.Contract.Object != knowledge.LiveAccessInterfaceV1 || implementation.ContractDigest != contract || implementation.ImplementationSchemaStructureDigest != schema {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess handler does not support the declared contract and implementation schema release")
	}
	return registry.Register(InterfaceRegistration{Contract: implementation.Contract, ContractDigest: contract, ImplementationSchemaStructureDigest: schema, HandlerRevision: "live-access-http/v1", Handler: NewLiveAccessHandler(accessor)})
}

// WithLiveAccess preserves installed extensions while adding the platform
// handler for one verified request basis. Shared registrations are never
// mutated by a consumer call, and unsupported custom contracts do not gain a
// fallback implementation.
func WithLiveAccess(installed *InterfaceRegistry, implementation reader.ResolvedInterface, accessor ResourceOperationAccessor) (*InterfaceRegistry, error) {
	if installed != nil {
		if _, err := installed.resolve(implementation); err == nil {
			return installed, nil
		}
	}
	registry := NewInterfaceRegistry()
	if installed != nil {
		installed.mu.RLock()
		registry.registrations = append(registry.registrations, installed.registrations...)
		installed.mu.RUnlock()
	}
	if err := RegisterLiveAccess(registry, implementation, accessor); err != nil {
		return nil, err
	}
	return registry, nil
}

func liveAccessPublicationBasis() (kernel.Digest, kernel.Digest, error) {
	var contract, schema kernel.Digest
	for _, operation := range knowledge.SystemPublicationOperations() {
		switch operation.Address.ObjectID {
		case knowledge.LiveAccessInterfaceV1:
			definition, err := knowledge.ParseInterfaceDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				return "", "", err
			}
			contract = definition.Digest
		case knowledge.LiveAccessImplementationSchemaV1:
			definition, err := knowledge.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value)
			if err != nil {
				return "", "", err
			}
			schema = knowledge.SchemaStructureDigest(definition)
		}
	}
	if contract == "" || schema == "" {
		return "", "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "LiveAccess publication is unavailable")
	}
	return contract, schema, nil
}

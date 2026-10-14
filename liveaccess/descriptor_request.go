package liveaccess

import (
	"strings"

	"kc/kernel"
)

// DescriptorOperation interprets the selected fixed descriptor. It neither
// resolves a different declaration nor invokes the source.
func DescriptorOperation(coordinate ResourceDescriptorCoordinate, raw any, operation string, input any, identity StateRuntimeIdentity) (ResourceOperationRequest, error) {
	value, ok := raw.(map[string]any)
	if !ok || value["kind"] != "ResourceDescriptor" {
		return ResourceOperationRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "object %s is not a ResourceDescriptor", coordinate.ObjectID)
	}
	runtime, protocol := strings.TrimSpace(asString(value["runtime"])), strings.TrimSpace(asString(value["protocol"]))
	origin := strings.TrimSpace(asString(value["origin"]))
	if origin == "" {
		return ResourceOperationRequest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "ResourceDescriptor %s does not declare origin", coordinate.ObjectID)
	}
	access, ok := value["access"].(map[string]any)
	if !ok || runtime == "" || protocol == "" {
		return ResourceOperationRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires runtime, protocol and access", coordinate.ObjectID)
	}
	selected, ok := access[operation].(map[string]any)
	if !ok {
		return ResourceOperationRequest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "ResourceDescriptor %s does not declare operation %s", coordinate.ObjectID, operation)
	}
	call := strings.TrimSpace(asString(selected["call"]))
	if call == "" {
		return ResourceOperationRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s operation %s requires call", coordinate.ObjectID, operation)
	}
	return ResourceOperationRequest{Descriptor: coordinate, Runtime: runtime, Protocol: protocol, Origin: origin, Operation: operation, Call: call, Input: input, Identity: identity}, nil
}

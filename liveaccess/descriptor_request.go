package liveaccess

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// DescriptorOperation interprets the selected fixed descriptor. It neither
// resolves a different declaration nor invokes the source.
func DescriptorOperation(coordinate ResourceDescriptorCoordinate, raw any, operation string, input any, identity StateRuntimeIdentity) (ResourceOperationRequest, error) {
	descriptor, err := objectmodel.ParseResourceDescriptor(coordinate.ObjectID, raw)
	if err != nil {
		return ResourceOperationRequest{}, err
	}
	selected, ok := descriptor.Access[operation]
	if !ok {
		return ResourceOperationRequest{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "ResourceDescriptor %s does not declare operation %s", coordinate.ObjectID, operation)
	}
	return ResourceOperationRequest{Descriptor: coordinate, Runtime: descriptor.Runtime, Protocol: descriptor.Protocol, Origin: descriptor.Origin, Operation: operation, Call: selected.Call, Input: input, Identity: identity}, nil
}

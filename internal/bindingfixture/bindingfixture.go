// Package bindingfixture builds LiveAccess declarations for tests: one
// ResourceDescriptor entity plus aspect/member units whose valueSource names
// it. It depends only on the object model so package-internal tests of any
// layer can use it without an import cycle.
package bindingfixture

import objectmodel "kc/object"

// Runtime is the runtime name fixtures declare when a test does not care.
const Runtime = "http"

// Descriptor returns a state ResourceDescriptor body with a single
// `lookup` operation.
func Descriptor(origin string) map[string]any {
	return DescriptorWith(origin, objectmodel.BindingState, Runtime, map[string]string{"lookup": "lookup"})
}

// StreamDescriptor returns a stream ResourceDescriptor body.
func StreamDescriptor(origin string) map[string]any {
	return DescriptorWith(origin, objectmodel.BindingStream, Runtime, map[string]string{"subscribe": "subscribe"})
}

// DescriptorWith returns a ResourceDescriptor body with explicit mode,
// runtime and operation calls (operation name -> call).
func DescriptorWith(origin string, mode objectmodel.BindingMode, runtime string, operations map[string]string) map[string]any {
	access := make(map[string]any, len(operations))
	for name, call := range operations {
		access[name] = map[string]any{"call": call}
	}
	return map[string]any{
		"kind":     objectmodel.ResourceDescriptorKind,
		"origin":   origin,
		"mode":     string(mode),
		"runtime":  runtime,
		"protocol": objectmodel.ProtocolResourceAccessV1,
		"access":   access,
	}
}

// PutDescriptor publishes a ResourceDescriptor entity.
func PutDescriptor(id objectmodel.ObjectID, body map[string]any) objectmodel.Operation {
	return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, Value: body}
}

// Source is the only valueSource shape: a binding naming a descriptor.
func Source(descriptor objectmodel.ObjectID) *objectmodel.ValueSource {
	return &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: descriptor}}
}

// PutAspect publishes an aspect unit whose value is supplied by descriptor.
func PutAspect(objectID objectmodel.ObjectID, aspect, schemaRef string, descriptor objectmodel.ObjectID) objectmodel.Operation {
	return Bind(objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectID, AspectName: aspect}, schemaRef, descriptor)
}

// PutMember publishes a member unit whose value is supplied by descriptor.
func PutMember(objectID objectmodel.ObjectID, aspect, member, schemaRef string, descriptor objectmodel.ObjectID) objectmodel.Operation {
	return Bind(objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: objectID, AspectName: aspect, MemberKey: member}, schemaRef, descriptor)
}

// Bind publishes a binding unit at address. Binding units carry no value.
func Bind(address objectmodel.Address, schemaRef string, descriptor objectmodel.ObjectID) objectmodel.Operation {
	return objectmodel.Operation{Op: objectmodel.OpPut, Address: address, SchemaRef: schemaRef, ValueSource: Source(descriptor)}
}

// WithPath places op at an explicit repository path.
func WithPath(op objectmodel.Operation, path string) objectmodel.Operation {
	op.PathHint = path
	return op
}

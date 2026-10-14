package invocation

import (
	interfaces "kc/liveaccess/handler"
	objectmodel "kc/object"

	"kc/observability"
)

// ObserveObjectInterfaceCall projects the known execution envelope into the
// existing access evidence. Business values and arbitrary handler basis remain
// opaque; only a typed state observation becomes a Member UnitObservation.
func ObserveObjectInterfaceCall(result interfaces.InterfaceCallResult, request interfaces.InterfaceRequest) ObservedAccessResult {
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: request.Object, AspectName: interfaces.ObjectInterfacesAspect, MemberKey: request.Implementation}
	implementation := observability.ObjectAccess{ObjectRef: objectmodel.PinnedObjectRef{ObjectRef: objectmodel.ObjectRef{Repository: request.Repository, Object: request.Object}, Commit: result.Basis.DeclarationCommit}, Address: &address}
	var observation *objectmodel.ObservationBasis
	switch basis := result.Observation.(type) {
	case objectmodel.ObservationBasis:
		observation = &basis
	case *objectmodel.ObservationBasis:
		observation = basis
	}
	if observation != nil {
		implementation.Observations = []objectmodel.UnitObservation{{Address: address, DeclarationCommit: result.Basis.DeclarationCommit, DeclarationDigest: result.Basis.DeclarationDigest, Basis: *observation}}
	}
	return ObservedAccessResult{Output: result, Object: []observability.ObjectAccess{implementation, {ObjectRef: result.Basis.Contract}, {ObjectRef: result.Basis.ImplementationSchema}}}
}

func objectInterfaceAuditRefs(result any) map[string]any {
	value, ok := accessOutput(result).(interfaces.InterfaceCallResult)
	if !ok {
		return nil
	}
	basis := value.Basis
	return map[string]any{"declarationCommit": basis.DeclarationCommit, "declarationDigest": basis.DeclarationDigest, "interfaceContract": basis.Contract, "contractDigest": basis.ContractDigest, "implementationSchema": basis.ImplementationSchema, "implementationSchemaDigest": basis.ImplementationSchemaDigest, "handlerRevision": basis.HandlerRevision}
}

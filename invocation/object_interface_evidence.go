package invocation

import (
	"kc/knowledge"
	"kc/knowledge/serving"
	"kc/observability"
)

// ObserveObjectInterfaceCall projects the known execution envelope into the
// existing access evidence. Business values and arbitrary handler basis remain
// opaque; only a typed state observation becomes a Member UnitObservation.
func ObserveObjectInterfaceCall(result serving.InterfaceCallResult, request serving.InterfaceRequest) ObservedAccessResult {
	address := knowledge.Address{Kind: knowledge.KindMember, ObjectID: request.Object, AspectName: knowledge.ObjectInterfacesAspect, MemberKey: request.Implementation}
	implementation := observability.KnowledgeAccess{KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: knowledge.KnowledgeRef{Repository: request.Repository, Object: request.Object}, Commit: result.Basis.DeclarationCommit}, Address: &address}
	var observation *knowledge.ObservationBasis
	switch basis := result.Observation.(type) {
	case knowledge.ObservationBasis:
		observation = &basis
	case *knowledge.ObservationBasis:
		observation = basis
	}
	if observation != nil {
		implementation.Observations = []knowledge.UnitObservation{{Address: address, DeclarationCommit: result.Basis.DeclarationCommit, DeclarationDigest: result.Basis.DeclarationDigest, Basis: *observation}}
	}
	return ObservedAccessResult{Output: result, Knowledge: []observability.KnowledgeAccess{implementation, {KnowledgeRef: result.Basis.Contract}, {KnowledgeRef: result.Basis.ImplementationSchema}}}
}

func objectInterfaceAuditRefs(result any) map[string]any {
	value, ok := accessOutput(result).(serving.InterfaceCallResult)
	if !ok {
		return nil
	}
	basis := value.Basis
	return map[string]any{"declarationCommit": basis.DeclarationCommit, "declarationDigest": basis.DeclarationDigest, "interfaceContract": basis.Contract, "contractDigest": basis.ContractDigest, "implementationSchema": basis.ImplementationSchema, "implementationSchemaDigest": basis.ImplementationSchemaDigest, "handlerRevision": basis.HandlerRevision}
}

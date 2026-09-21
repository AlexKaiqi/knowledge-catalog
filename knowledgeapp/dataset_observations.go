package knowledgeapp

import (
	"kc/kernel"
	"kc/knowledge/reader"
	"kc/retrieval"
)

// A repository-wide State cache is not a Dataset grant. Validate every
// observation's declaration in the accepted file scope before delivery; never
// re-observe the runtime to repair a cache whose basis does not match.
func validateDatasetObservations(scope *reader.Serving, hit retrieval.KnowledgeHit) error {
	for _, observation := range hit.Version.Observations {
		if observation.Address.ObjectID != hit.Knowledge.KnowledgeRef.Object {
			return kernel.Fail(kernel.ErrPreconditionFailed, "State observation belongs to another object")
		}
		binding, err := scope.ResolveBindingAt(hit.Knowledge.Repository, observation.Address)
		if err != nil {
			return kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "State declaration is outside dataset scope or unavailable")
		}
		if binding.DeclarationCommit != observation.DeclarationCommit || binding.DeclarationDigest != observation.DeclarationDigest || binding.DescriptorDigest != observation.DescriptorDigest {
			return kernel.Fail(kernel.ErrPreconditionFailed, "State observation does not match dataset declaration")
		}
	}
	return nil
}

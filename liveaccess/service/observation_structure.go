package service

import (
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
)

func (s *Service) validateObservedStructure(binding liveaccess.ResolvedBinding, value any) error {
	// Observed null proves an empty observation. It must not be substituted
	// with a fabricated object or confused with an unperformed lookup.
	if value == nil || binding.SchemaRef == "" {
		return nil
	}
	ref, ok := objectmodel.ParseSchemaRef(binding.SchemaRef)
	if !ok || (ref.Repository != "" && ref.Repository != binding.Repository) {
		return kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "Binding has no valid adopted Schema")
	}
	repo, err := s.interpretationRepository(binding.Repository)
	if err != nil {
		return err
	}
	at := ref.Commit
	if at == "" {
		at = binding.DeclarationCommit
	}
	definition, err := repo.Read(ref.Object, at)
	if err != nil {
		return err
	}
	schema, err := objectmodel.ParseSchemaDefinition(ref.Object, definition.Value)
	if err != nil {
		return err
	}
	if schema.Bound() && objectmodel.BoundDeclarationDigest(schema) != binding.DeclarationDigest {
		return kernel.Fail(kernel.ErrPreconditionFailed, "observation Schema does not match its fixed Binding declaration")
	}
	return objectmodel.ValidateSchemaInstance(binding.Address, value, schema)
}

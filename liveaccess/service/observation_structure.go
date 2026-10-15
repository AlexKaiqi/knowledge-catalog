package service

import (
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
)

// observedSchema resolves the adopted Schema that will validate an
// observation. It runs before the source lookup so an uninterpretable
// declaration fails closed without contacting the source.
func (s *Service) observedSchema(binding liveaccess.ResolvedBinding) (*objectmodel.SchemaDefinition, error) {
	if binding.SchemaRef == "" {
		return nil, nil
	}
	ref, ok := objectmodel.ParseSchemaRef(binding.SchemaRef)
	if !ok || (ref.Repository != "" && ref.Repository != binding.Repository) {
		return nil, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "Binding has no valid adopted Schema")
	}
	repo, err := s.interpretationRepository(binding.Repository)
	if err != nil {
		return nil, err
	}
	at := ref.Commit
	if at == "" {
		at = binding.DeclarationCommit
	}
	definition, err := repo.Read(ref.Object, at)
	if err != nil {
		return nil, err
	}
	schema, err := objectmodel.ParseSchemaDefinition(ref.Object, definition.Value)
	if err != nil {
		return nil, err
	}
	return &schema, nil
}

func validateObservedStructure(address objectmodel.Address, schema *objectmodel.SchemaDefinition, value any) error {
	// Observed null proves an empty observation. It must not be substituted
	// with a fabricated object or confused with an unperformed lookup.
	if value == nil || schema == nil {
		return nil
	}
	return objectmodel.ValidateSchemaInstance(address, value, *schema)
}

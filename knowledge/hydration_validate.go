package knowledge

import "kc/kernel"

// ValidateHydratedObject verifies that an injected hydration result represents
// the complete object at the requested immutable coordinates, not a single
// Aspect or Member with the same object identity.
func ValidateHydratedObject(repository kernel.RepositoryID, commit kernel.CommitID, id ObjectID, value KnowledgeValue) error {
	if err := validateHydratedIdentity(repository, commit, id, value); err != nil {
		return err
	}
	if value.Address.AspectName != "" || value.Address.MemberKey != "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "hydration returned a unit instead of a whole object")
	}
	return nil
}

// ValidateHydratedAddress verifies the immutable identity and exact unit
// coordinates. Kind remains Canonical metadata rather than a separate address
// identity, matching Repository Address reads.
func ValidateHydratedAddress(repository kernel.RepositoryID, commit kernel.CommitID, address Address, value KnowledgeValue) error {
	if err := validateHydratedIdentity(repository, commit, address.ObjectID, value); err != nil {
		return err
	}
	if value.Address.AspectName != address.AspectName || value.Address.MemberKey != address.MemberKey {
		return kernel.Fail(kernel.ErrPreconditionFailed, "hydration returned another Address")
	}
	return nil
}

func validateHydratedIdentity(repository kernel.RepositoryID, commit kernel.CommitID, id ObjectID, value KnowledgeValue) error {
	if repository == "" || commit == "" || id == "" || value.Repository != repository || value.Commit != commit ||
		value.KnowledgeRef.Repository != repository || value.KnowledgeRef.Object != id || value.Address.ObjectID != id {
		return kernel.Fail(kernel.ErrPreconditionFailed, "hydration does not match the fixed repository, commit and object")
	}
	for _, address := range value.Units {
		if address.ObjectID != id {
			return kernel.Fail(kernel.ErrPreconditionFailed, "hydration returned a foreign unit")
		}
	}
	for _, declaration := range value.Declarations {
		if declaration.Address.ObjectID != id {
			return kernel.Fail(kernel.ErrPreconditionFailed, "hydration returned a foreign declaration")
		}
	}
	return nil
}

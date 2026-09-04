package knowledge

import (
	"kc/kernel"
)

// AssertSourceProfileBinding keeps the platform source-profile envelope on
// its reserved Address. Other object IDs may not adopt this Schema; the
// reserved object may not use another Schema.
func AssertSourceProfileBinding(address Address, schemaObjectID ObjectID) error {
	reserved := address.ObjectID == SourceProfileObjectID
	if reserved {
		if address.Kind != KindEntity || address.AspectName != "" || address.MemberKey != "" {
			return schemaInstanceInvalid(address, "source profile must be the reserved entity address %s", SourceProfileObjectID)
		}
		if schemaObjectID != CoreSourceProfileSchemaV1 {
			return schemaInstanceInvalid(address, "source profile must use %s", CoreSourceProfileSchemaV1)
		}
		return nil
	}
	if schemaObjectID == CoreSourceProfileSchemaV1 {
		return schemaInstanceInvalid(address, "schema %s is reserved for %s", CoreSourceProfileSchemaV1, SourceProfileObjectID)
	}
	return nil
}

// AssertProtocolSchemaPublication refuses a drifted copy of a platform Schema
// object ID. Business Repositories may republish the same document so
// schema_ref stays in-repo; they may not evolve it in place.
func AssertProtocolSchemaPublication(objectID ObjectID, value any) error {
	want, ok := protocolSchemaDigest(objectID)
	if !ok {
		return nil
	}
	if kernel.CanonicalDigest(value) != want {
		return kernel.Fail(kernel.ErrSchemaIncompatible,
			"protocol schema %s must match the System Repository publication", objectID)
	}
	return nil
}

func protocolSchemaDigest(objectID ObjectID) (kernel.Digest, bool) {
	if objectID != CoreSourceProfileSchemaV1 {
		return "", false
	}
	for _, operation := range SystemSchemaOperations() {
		if operation.Address.ObjectID == objectID {
			return kernel.CanonicalDigest(operation.Value), true
		}
	}
	return "", false
}

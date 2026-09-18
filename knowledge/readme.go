package knowledge

import (
	"strings"

	"kc/kernel"
)

// IsReadmeSchema reports whether schema_ref names the protocol README Aspect.
func IsReadmeSchema(schemaRef string) bool {
	parsed, ok := ParseSchemaRef(schemaRef)
	return ok && parsed.Object == CoreReadmeSchemaV1
}

// IsReadmeAddress is the Address shape required by schema/core/readme/v1.
func IsReadmeAddress(address Address, schemaRef string) bool {
	return IsReadmeSchema(schemaRef) &&
		address.Kind == KindAspect &&
		address.AspectName == ReadmeAspect &&
		address.MemberKey == ""
}

// AssertReadmeBinding keeps the protocol README Schema on Aspect `readme`.
// Any entity may own that Aspect; the Aspect name is not free for other Schemas.
func AssertReadmeBinding(address Address, schemaObjectID ObjectID) error {
	readmeAddress := address.Kind == KindAspect && address.AspectName == ReadmeAspect && address.MemberKey == ""
	if schemaObjectID == CoreReadmeSchemaV1 {
		if !readmeAddress {
			return schemaInstanceInvalid(address, "schema %s requires aspect %s", CoreReadmeSchemaV1, ReadmeAspect)
		}
		return nil
	}
	if readmeAddress {
		return schemaInstanceInvalid(address, "aspect %s requires schema %s", ReadmeAspect, CoreReadmeSchemaV1)
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
	if objectID != CoreReadmeSchemaV1 {
		return "", false
	}
	for _, operation := range SystemSchemaOperations() {
		if operation.Address.ObjectID == objectID {
			return kernel.CanonicalDigest(operation.Value), true
		}
	}
	return "", false
}

// ReadmeBody extracts the markdown field from a README unit or assembled object.
func ReadmeBody(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", false
		}
		return typed, true
	case map[string]any:
		if raw, ok := typed["body"].(string); ok && strings.TrimSpace(raw) != "" {
			return raw, true
		}
		if nested, ok := typed[ReadmeAspect]; ok {
			return ReadmeBody(nested)
		}
	}
	return "", false
}

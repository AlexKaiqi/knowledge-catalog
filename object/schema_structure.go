package object

import "kc/kernel"

// SchemaStructureDigest identifies a normalized schema for handler support
// matching. Schema describes structure only; retrieval capability lives in a
// separate index declaration and never enters the Schema, so the structure
// digest now covers the complete normalized definition. This does not replace
// the complete schema digest and pinned reference retained as object and
// execution evidence.
func SchemaStructureDigest(definition SchemaDefinition) kernel.Digest {
	return kernel.CanonicalDigest(definition)
}

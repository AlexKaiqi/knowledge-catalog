package object

import "kc/kernel"

// SchemaStructureDigest identifies a normalized schema for handler support
// matching, excluding only field query access declarations. Every other schema
// semantic remains part of the match. This does not replace the complete schema
// digest and pinned reference retained as knowledge and execution evidence.
func SchemaStructureDigest(definition SchemaDefinition) kernel.Digest {
	structure := definition
	if definition.Fields != nil {
		structure.Fields = make([]SchemaFieldDefinition, len(definition.Fields))
		copy(structure.Fields, definition.Fields)
	}
	for i := range structure.Fields {
		structure.Fields[i].Access = nil
	}
	return kernel.CanonicalDigest(structure)
}

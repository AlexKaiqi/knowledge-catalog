package testkit

import objectmodel "kc/object"

// IndexDeclarationObject holds fixture index declarations. A declaration is an
// ordinary object's `index` aspect; fixtures keep theirs under one object.
const IndexDeclarationObject = "index/fixture"

// IndexDeclarationCatalog is the compiled declarative index manifest fixtures
// name. It matches the plugin entry's default instance.
const IndexDeclarationCatalog = "declarative-index/opensearch@2026-10"

// IndexMember is one index declaration member fixture: one retrieval field
// built as kind over the named Schema fields, serving uses.
//
// Args:
//
//	member: stable member key.
//	kind: manifest build kind (text, keyword, number, date, boolean, vector).
//	uses: text, filter, sort or semantic.
//	from: `<schema-ref>#/fields/<path>` input locators.
func IndexMember(member, kind string, uses []string, from ...string) objectmodel.Operation {
	inputs := make([]any, len(from))
	for i, locator := range from {
		inputs[i] = map[string]any{"from": locator}
	}
	declared := make([]any, len(uses))
	for i, use := range uses {
		declared[i] = use
	}
	return objectmodel.Operation{
		Op:        objectmodel.OpPut,
		Address:   objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: IndexDeclarationObject, AspectName: "index", MemberKey: member},
		SchemaRef: string(objectmodel.IndexDeclarationSchemaV1),
		Value:     map[string]any{"catalog": IndexDeclarationCatalog, "inputs": inputs, "build": map[string]any{"kind": kind}, "uses": declared},
	}
}

// IndexText declares full-text retrieval over one Schema string field.
func IndexText(schema objectmodel.ObjectID, path string) objectmodel.Operation {
	return IndexMember(memberKey(schema, path, "text"), "text", []string{objectmodel.IndexUseText}, fieldLocator(schema, path))
}

// IndexField declares typed filter (and sort) over one Schema field. kind is
// the manifest build kind matching the field type.
func IndexField(schema objectmodel.ObjectID, path, kind string, uses ...string) objectmodel.Operation {
	if len(uses) == 0 {
		uses = []string{objectmodel.IndexUseFilter}
	}
	return IndexMember(memberKey(schema, path, kind), kind, uses, fieldLocator(schema, path))
}

func fieldLocator(schema objectmodel.ObjectID, path string) string {
	return string(schema) + "#/fields/" + path
}

// IndexUnder places a declaration member file under a directory, so a Dataset
// selecting that prefix also selects the declaration. Only declarations
// selected into a Dataset take effect there.
func IndexUnder(prefix string, op objectmodel.Operation) objectmodel.Operation {
	op.PathHint = prefix + "/" + IndexDeclarationObject + "/" + op.Address.MemberKey + ".yaml"
	return op
}

// IndexOn keeps a declaration member on the named declaration object instead
// of IndexDeclarationObject. Sources composed into one Dataset must not hold
// the same object, so each source declares on its own object.
func IndexOn(object objectmodel.ObjectID, op objectmodel.Operation) objectmodel.Operation {
	op.Address.ObjectID = object
	return op
}

func memberKey(schema objectmodel.ObjectID, path, kind string) string {
	key := []byte(string(schema) + "-" + path + "-" + kind)
	for i, b := range key {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_') {
			key[i] = '-'
		}
	}
	return string(key)
}

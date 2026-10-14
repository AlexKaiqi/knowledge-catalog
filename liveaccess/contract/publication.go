package contract

import (
	"embed"
	objectmodel "kc/object"
	"kc/object/format"
)

//go:embed trusted/*.yaml
var standardPublication embed.FS

// StandardPublicationOperations is the standard LiveAccess contract and
// declaration Schema, independent of the complete product self-description.
func StandardPublicationOperations() []objectmodel.Operation {
	entries, err := standardPublication.ReadDir("trusted")
	if err != nil {
		panic(err)
	}
	out := make([]objectmodel.Operation, 0, len(entries))
	for _, entry := range entries {
		raw, err := standardPublication.ReadFile("trusted/" + entry.Name())
		if err != nil {
			panic(err)
		}
		unit := format.Parse(string(raw))
		if unit == nil || unit.DeclarationError() != nil {
			panic("invalid LiveAccess standard publication")
		}
		out = append(out, objectmodel.Operation{Op: objectmodel.OpPut, Address: unit.Address, PathHint: unit.PathHint, SchemaRef: unit.SchemaRef, Value: unit.Value})
	}
	return out
}

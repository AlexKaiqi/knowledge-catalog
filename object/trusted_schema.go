package object

import (
	"embed"
	"strings"

	"gopkg.in/yaml.v3"
	"kc/kernel"
)

//go:embed trusted/*.yaml
var trustedSchemaFS embed.FS

// TrustedSchemaOperations contains the immutable Meta Schema and index
// declaration protocol data. Basic object read/write never imports the system publication.
func TrustedSchemaOperations() []Operation {
	entries, err := trustedSchemaFS.ReadDir("trusted")
	if err != nil {
		panic(err)
	}
	out := make([]Operation, 0, len(entries))
	for _, entry := range entries {
		raw, err := trustedSchemaFS.ReadFile("trusted/" + entry.Name())
		if err != nil {
			panic(err)
		}
		header, body, ok := strings.Cut(strings.TrimPrefix(string(raw), "---\n"), "\n---\n")
		if !ok {
			panic("trusted Schema has no declaration")
		}
		var declaration struct {
			ID   ObjectID `yaml:"object_id"`
			Path string   `yaml:"path_hint"`
		}
		if err := yaml.Unmarshal([]byte(header), &declaration); err != nil {
			panic(err)
		}
		var value any
		if err := yaml.Unmarshal([]byte(body), &value); err != nil {
			panic(err)
		}
		out = append(out, Operation{Op: OpPut, Address: Address{Kind: KindEntity, ObjectID: declaration.ID}, PathHint: declaration.Path, Value: value})
	}
	return out
}

// AssertProtocolSchemaPublication refuses drifted copies of every protocol
// Schema in TrustedSchemaOperations. Repositories may republish those documents
// unchanged; other Schema IDs retain ordinary compatible evolution.
func AssertProtocolSchemaPublication(objectID ObjectID, value any) error {
	for _, operation := range TrustedSchemaOperations() {
		if operation.Address.ObjectID == objectID {
			if kernel.CanonicalDigest(value) != kernel.CanonicalDigest(operation.Value) {
				return kernel.Fail(kernel.ErrSchemaIncompatible,
					"protocol schema %s must match the System Repository publication", objectID)
			}
			return nil
		}
	}
	return nil
}

func SystemMetaSchemaDigest() kernel.Digest {
	for _, operation := range TrustedSchemaOperations() {
		if operation.Address.ObjectID == MetaSchemaV1 {
			return kernel.CanonicalDigest(operation.Value)
		}
	}
	panic("Object trust root has no Meta Schema")
}

// ProtocolTrustRootSchema returns a protocol Schema that resolves from the
// embedded trust root rather than from the adopting Repository. Only the
// index declaration Schema is such a root: declarations live in a Dataset
// maintainer's repository, which publishes no copy of the protocol Schema.
func ProtocolTrustRootSchema(objectID ObjectID) (SchemaDefinition, bool) {
	if objectID != IndexDeclarationSchemaV1 {
		return SchemaDefinition{}, false
	}
	for _, operation := range TrustedSchemaOperations() {
		if operation.Address.ObjectID == objectID {
			definition, err := ParseSchemaDefinition(objectID, operation.Value)
			if err != nil {
				panic(err)
			}
			return definition, true
		}
	}
	panic("Object trust root has no index declaration Schema")
}

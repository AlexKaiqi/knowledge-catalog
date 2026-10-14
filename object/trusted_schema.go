package object

import (
	"embed"
	"strings"

	"gopkg.in/yaml.v3"
	"kc/kernel"
)

//go:embed trusted/*.yaml
var trustedSchemaFS embed.FS

// TrustedSchemaOperations contains the immutable Meta Schema and README
// protocol data. Basic object read/write never imports the system publication.
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

func SystemMetaSchemaDigest() kernel.Digest {
	for _, operation := range TrustedSchemaOperations() {
		if operation.Address.ObjectID == MetaSchemaV1 {
			return kernel.CanonicalDigest(operation.Value)
		}
	}
	panic("Object trust root has no Meta Schema")
}

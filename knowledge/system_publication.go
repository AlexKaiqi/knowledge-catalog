package knowledge

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"kc/kernel"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed all:system
var systemPublicationFS embed.FS

var systemPublicationOnce sync.Once
var systemPublication []Operation

// SystemPublicationOperations returns independent values for the complete,
// immutable self-description shipped with this binary, including its Schemas.
func SystemPublicationOperations() []Operation {
	units := systemPublicationUnits()
	out := make([]Operation, len(units))
	for i, operation := range units {
		out[i] = cloneSystemOperation(operation)
	}
	return out
}

func systemPublicationUnits() []Operation {
	systemPublicationOnce.Do(func() {
		if err := fs.WalkDir(systemPublicationFS, "system", func(name string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			raw, err := systemPublicationFS.ReadFile(name)
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(name, "system/")
			if rel == "readme.md" {
				rel = RepositoryReadmePath
			}
			operation, err := decodeSystemPublication(rel, string(raw))
			if err != nil {
				return fmt.Errorf("system publication %s: %w", name, err)
			}
			systemPublication = append(systemPublication, operation)
			return nil
		}); err != nil {
			panic(err)
		}
	})
	return systemPublication
}

func cloneSystemOperation(operation Operation) Operation {
	value, err := cloneSystemValue(operation.Value)
	if err != nil {
		panic(err)
	}
	operation.Value = value
	return operation
}

// The embedded publication is an authored Canonical tree, not an alternate
// ingest surface. Its decoding is checked against the normal file codec by
// TestSystemPublicationMatchesCanonicalFilesAndSchemas.
func decodeSystemPublication(rel, raw string) (Operation, error) {
	header, content, ok := strings.Cut(strings.TrimPrefix(raw, "---\n"), "\n---\n")
	if !strings.HasPrefix(raw, "---\n") || !ok {
		return Operation{}, fmt.Errorf("missing Canonical frontmatter")
	}
	var declaration struct {
		ObjectID  ObjectID `yaml:"object_id"`
		Entity    ObjectID `yaml:"entity"`
		Aspect    string   `yaml:"aspect_name"`
		Alias     string   `yaml:"aspect"`
		Kind      string   `yaml:"kind"`
		SchemaRef string   `yaml:"schema_ref"`
		Path      string   `yaml:"path_hint"`
	}
	if err := yaml.Unmarshal([]byte(header), &declaration); err != nil {
		return Operation{}, err
	}
	if declaration.ObjectID == "" {
		declaration.ObjectID = declaration.Entity
	}
	if declaration.Aspect == "" {
		declaration.Aspect = declaration.Alias
	}
	if declaration.ObjectID == "" || declaration.Path != rel {
		return Operation{}, fmt.Errorf("requires object_id and matching path_hint")
	}
	address := InferAddress(declaration.ObjectID, declaration.Aspect, "", declaration.Kind)
	if err := AssertWritable(address); err != nil {
		return Operation{}, err
	}
	var value any
	if strings.HasSuffix(rel, ".md") {
		value = map[string]any{"body": strings.TrimSpace(content)}
	} else if err := yaml.Unmarshal([]byte(content), &value); err != nil {
		return Operation{}, err
	}
	return Operation{Op: OpPut, Address: address, PathHint: rel, SchemaRef: declaration.SchemaRef, Value: value}, nil
}

// SystemSchemaOperations returns fresh values so callers cannot mutate the
// process trust root. Paths are presentation/storage hints; object IDs remain
// the canonical identities. The publication source is the embedded Canonical
// tree under system/; the digest is computed from parsed values.
func SystemSchemaOperations() []Operation {
	var out []Operation
	for _, operation := range systemPublicationUnits() {
		if IsSchemaObject(operation.Address.ObjectID) {
			out = append(out, cloneSystemOperation(operation))
		}
	}
	return out
}

// SystemReadmeOperation is the System Repository description: a README Aspect
// whose identity lives in markdown frontmatter.
func SystemReadmeOperation() Operation {
	for _, operation := range systemPublicationUnits() {
		if operation.PathHint == RepositoryReadmePath {
			return cloneSystemOperation(operation)
		}
	}
	panic("System Repository must publish README.md")
}

func SystemMetaSchemaDigest() kernel.Digest {
	for _, operation := range SystemSchemaOperations() {
		if operation.Address.ObjectID == MetaSchemaV1 {
			return kernel.CanonicalDigest(operation.Value)
		}
	}
	panic("System Repository must publish the Meta Schema")
}

func cloneSystemValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

package systemrepo

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	liveaccesscontract "kc/liveaccess/contract"
	objectmodel "kc/object"
	"strings"
	"sync"

	"kc/object/format"
)

//go:embed all:content
var systemPublicationFS embed.FS

var systemPublicationOnce sync.Once
var systemPublication []objectmodel.Operation

// SystemPublicationOperations returns independent values for the complete,
// immutable self-description shipped with this binary, including its Schemas.
func SystemPublicationOperations() []objectmodel.Operation {
	units := systemPublicationUnits()
	out := make([]objectmodel.Operation, len(units))
	for i, operation := range units {
		out[i] = cloneSystemOperation(operation)
	}
	return out
}

func systemPublicationUnits() []objectmodel.Operation {
	systemPublicationOnce.Do(func() {
		systemPublication = append(systemPublication, objectmodel.TrustedSchemaOperations()...)
		systemPublication = append(systemPublication, liveaccesscontract.StandardPublicationOperations()...)
		if err := fs.WalkDir(systemPublicationFS, "content", func(name string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			raw, err := systemPublicationFS.ReadFile(name)
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(name, "content/")
			if rel == "readme.md" {
				rel = systemReadmePath
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

func cloneSystemOperation(operation objectmodel.Operation) objectmodel.Operation {
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
func decodeSystemPublication(rel, raw string) (objectmodel.Operation, error) {
	unit := format.Parse(raw)
	if unit == nil || unit.ObjectID == "" || unit.PathHint != rel {
		return objectmodel.Operation{}, fmt.Errorf("requires object_id and matching path_hint")
	}
	if err := unit.DeclarationError(); err != nil {
		return objectmodel.Operation{}, err
	}
	return objectmodel.Operation{Op: objectmodel.OpPut, Address: unit.Address, PathHint: rel, SchemaRef: unit.SchemaRef, Value: unit.Value}, nil
}

// SystemSchemaOperations returns fresh values so callers cannot mutate the
// process trust root. Paths are presentation/storage hints; object IDs remain
// the canonical identities. The publication source is the embedded Canonical
// tree under system/; the digest is computed from parsed values.
func SystemSchemaOperations() []objectmodel.Operation {
	var out []objectmodel.Operation
	for _, operation := range systemPublicationUnits() {
		if objectmodel.IsSchemaObject(operation.Address.ObjectID) {
			out = append(out, cloneSystemOperation(operation))
		}
	}
	return out
}

// systemReadmePath is where the System Repository keeps its own guide. The
// embedded source is lower-case so the package tree does not mistake it for a
// package README; the published path carries no protocol meaning.
const systemReadmePath = "README.md"

// SystemReadmeOperation is the System Repository's guide: an ordinary
// readme Aspect on entity kc/system.
func SystemReadmeOperation() objectmodel.Operation {
	for _, operation := range systemPublicationUnits() {
		if operation.PathHint == systemReadmePath {
			return cloneSystemOperation(operation)
		}
	}
	panic("System Repository must publish README.md")
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

package format

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// Apply compiles one maintained unit operation using the common object algebra.
func Apply(idx *Tree, op objectmodel.Operation, prov *objectmodel.ProvenanceEnvelope, toWrite map[string]string, toDelete map[string]struct{}) error {
	return ApplyOperations(idx, []objectmodel.Operation{op}, prov, toWrite, toDelete)
}

// ApplyOperations validates the complete final object representation before
// compiling its changed units into literal file writes and deletions.
func ApplyOperations(idx *Tree, operations []objectmodel.Operation, prov *objectmodel.ProvenanceEnvelope, toWrite map[string]string, toDelete map[string]struct{}) error {
	existing := make([]Unit, 0, len(idx.Units))
	for _, unit := range idx.Units {
		existing = append(existing, unit)
	}
	final, deleted, err := objectmodel.ApplyUnits(ToObjectUnits(existing), operations, prov)
	if err != nil {
		return err
	}
	removed := map[string]bool{}
	for _, address := range deleted {
		removed[objectmodel.AddressKey(address)] = true
	}
	puts := map[string]bool{}
	for _, op := range operations {
		if op.Op == objectmodel.OpPut {
			puts[objectmodel.AddressKey(op.Address)] = true
		}
	}
	encoded := make([]Unit, 0, len(puts))
	contents := make(map[string]string, len(puts))
	for _, unit := range final {
		key := objectmodel.AddressKey(unit.Address)
		if !puts[key] {
			continue
		}
		previous, exists := idx.Units[key]
		file, content, err := encodeChangedUnit(unit, previous, exists && !removed[key])
		if err != nil {
			return err
		}
		encoded = append(encoded, file)
		contents[file.Path] = content
	}
	for _, address := range deleted {
		if previous := idx.Remove(address); previous != nil {
			toDelete[previous.Path] = struct{}{}
			delete(toWrite, previous.Path)
		}
	}
	for _, file := range encoded {
		if previous, exists := idx.Units[objectmodel.AddressKey(file.Address)]; exists && previous.Path != file.Path {
			toDelete[previous.Path] = struct{}{}
			delete(toWrite, previous.Path)
		}
		toWrite[file.Path] = contents[file.Path]
		idx.Upsert(file)
	}
	return nil
}

func encodeChangedUnit(unit objectmodel.Unit, previous Unit, keepPosition bool) (Unit, string, error) {
	path := unit.PathHint
	if path == "" && keepPosition {
		path = previous.Path
	}
	if path == "" {
		path = DefaultPath(unit.Address, unit.SchemaRef)
	}
	path, err := SafeRelativePath(path)
	if err != nil {
		return Unit{}, "", err
	}
	if !ObjectPath(path) {
		return Unit{}, "", kernel.Fail(kernel.ErrUsageInvalid, "path must use a readable object file extension (.json, .md, .okf, .yaml, .yml, .txt): %s", path)
	}
	file := Unit{
		ObjectID: unit.ObjectID, Address: unit.Address, PathHint: unit.PathHint,
		SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource,
		Provenance: unit.Provenance, Value: unit.Value, Digest: unit.Digest, Path: path,
	}
	if file.PathHint != "" {
		file.PathHint = path
	}
	content, err := SerializeWithSource(file.Address, file.PathHint, file.SchemaRef, file.ValueSource, file.Provenance, file.Value)
	return file, content, err
}

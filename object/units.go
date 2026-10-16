// Package unitcodec owns provider-neutral unit mutation and assembly. File
// repositories add frontmatter/path serialization around it; native providers
// persist the same units as typed rows.
package object

import (
	"sort"

	"kc/kernel"
)

// Unit is the provider-neutral object atom. It deliberately has no storage
// path, filename, extension, or serialized-file field.
type Unit struct {
	ObjectID    ObjectID
	Address     Address
	PathHint    string
	SchemaRef   string
	ValueSource *ValueSource
	Provenance  *ProvenanceEnvelope
	Value       any
	Digest      kernel.Digest
}

func AssembleUnits(units []Unit) (any, error) {
	if len(units) == 0 {
		return nil, nil
	}
	var blobs, parts []Unit
	for _, unit := range units {
		if IsEntityBlob(unit.Address) {
			blobs = append(blobs, unit)
		} else {
			parts = append(parts, unit)
		}
	}
	if len(blobs) > 0 && len(parts) > 0 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", units[0].ObjectID)
	}
	if len(blobs) > 1 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "duplicate object_id %s", blobs[0].ObjectID)
	}
	if len(blobs) == 1 {
		return blobs[0].Value, nil
	}
	records := map[string]struct{}{}
	memberAspects := map[string]struct{}{}
	out := map[string]any{}
	members := map[string]map[string]any{}
	for _, unit := range parts {
		name := unit.Address.AspectName
		if name == "" {
			continue
		}
		if unit.Address.MemberKey == "" {
			records[name] = struct{}{}
			out[name] = unit.Value
			continue
		}
		memberAspects[name] = struct{}{}
		if members[name] == nil {
			members[name] = map[string]any{}
		}
		members[name][unit.Address.MemberKey] = unit.Value
	}
	for name := range memberAspects {
		if _, conflict := records[name]; conflict {
			return nil, kernel.Fail(kernel.ErrObjectIDConflict, "aspect %s is both Record and Member", name)
		}
		out[name] = members[name]
	}
	return out, nil
}

func DeclareUnits(units []Unit) []UnitDeclaration {
	sorted := append([]Unit(nil), units...)
	sort.Slice(sorted, func(i, j int) bool {
		return AddressKey(sorted[i].Address) < AddressKey(sorted[j].Address)
	})
	out := make([]UnitDeclaration, 0, len(sorted))
	for _, unit := range sorted {
		out = append(out, UnitDeclaration{
			Address: unit.Address, Digest: unit.Digest,
			DeclarationDigest: DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource,
			Provenance: unit.Provenance,
		})
	}
	return out
}

func UnitSetDeclarationDigest(units []Unit) kernel.Digest {
	declarations := DeclareUnits(units)
	rows := make([]any, 0, len(declarations))
	for _, declaration := range declarations {
		rows = append(rows, map[string]any{
			"address": AddressKey(declaration.Address),
			"digest":  declaration.DeclarationDigest,
		})
	}
	return kernel.CanonicalDigest(rows)
}

// ApplyUnits mutates only the supplied object working set. Callers load the units
// for object IDs touched by operations; no repository-wide tree is required.
func ApplyUnits(existing []Unit, operations []Operation, provenance *ProvenanceEnvelope) (final []Unit, deleted []Address, err error) {
	units := map[string]Unit{}
	// byID indexes unit keys per object so sibling lookups stay O(siblings)
	// instead of scanning every unit of the batch for every operation.
	byID := map[ObjectID]map[string]struct{}{}
	track := func(key string, id ObjectID) {
		keys := byID[id]
		if keys == nil {
			keys = map[string]struct{}{}
			byID[id] = keys
		}
		keys[key] = struct{}{}
	}
	untrack := func(key string, id ObjectID) {
		if keys := byID[id]; keys != nil {
			delete(keys, key)
			if len(keys) == 0 {
				delete(byID, id)
			}
		}
	}
	for _, unit := range existing {
		key := AddressKey(unit.Address)
		if previous, ok := units[key]; ok {
			untrack(key, previous.ObjectID)
		}
		units[key] = unit
		track(key, unit.ObjectID)
	}
	for _, operation := range operations {
		if operation.Op != OpPut && operation.Op != OpRemove {
			return nil, nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown object operation %s", operation.Op)
		}
		if err := AssertWritable(operation.Address); err != nil {
			return nil, nil, err
		}
		key := AddressKey(operation.Address)
		current, exists := units[key]
		if operation.Op == OpRemove {
			if IsEntityBlob(operation.Address) {
				siblingKeys := byID[operation.Address.ObjectID]
				if len(siblingKeys) == 0 {
					return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
						"object %s does not exist", operation.Address.ObjectID)
				}
				if operation.Precondition != nil && operation.Precondition.Digest != "" {
					siblings := make([]Unit, 0, len(siblingKeys))
					for siblingKey := range siblingKeys {
						siblings = append(siblings, units[siblingKey])
					}
					digest, err := UnitSetMaintenanceDigest(siblings)
					if err != nil {
						return nil, nil, err
					}
					if digest != operation.Precondition.Digest {
						return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
							"digest mismatch for %s", operation.Address.ObjectID)
					}
				}
				for siblingKey := range siblingKeys {
					deleted = append(deleted, units[siblingKey].Address)
					delete(units, siblingKey)
				}
				delete(byID, operation.Address.ObjectID)
				continue
			}
			if !exists {
				return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
					"address %s does not exist", key)
			}
			if operation.Precondition != nil && operation.Precondition.Digest != "" &&
				operation.Precondition.Digest != MaintenanceDigest(current.Digest, current.SchemaRef, current.ValueSource) {
				return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
					"digest mismatch for %s", key)
			}
			deleted = append(deleted, current.Address)
			delete(units, key)
			untrack(key, current.ObjectID)
			continue
		}
		if operation.Precondition != nil {
			switch operation.Precondition.Type {
			case IfAbsent:
				if exists {
					return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
						"address %s already exists", key)
				}
			case IfObjectEquals, IfDigestEquals:
				if operation.Precondition.Digest != "" &&
					(!exists || MaintenanceDigest(current.Digest, current.SchemaRef, current.ValueSource) != operation.Precondition.Digest) {
					return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
						"digest mismatch for %s", key)
				}
			}
		}
		schemaRef := operation.SchemaRef
		if schemaRef == "" && exists {
			schemaRef = current.SchemaRef
		}
		pathHint := operation.PathHint
		if pathHint == "" && exists {
			pathHint = current.PathHint
		}
		// A PUT carrying a value publishes a Snapshot unit; only a value-less
		// PUT keeps the existing binding declaration.
		source := operation.ValueSource
		if source == nil && exists && operation.Value == nil {
			source = current.ValueSource
		} else {
			source = source.Normalized()
		}
		envelope := provenance
		if envelope == nil && exists {
			envelope = current.Provenance
		}
		if exists {
			untrack(key, current.ObjectID)
		}
		units[key] = Unit{
			ObjectID: operation.Address.ObjectID, Address: operation.Address,
			PathHint: pathHint, SchemaRef: schemaRef, ValueSource: source,
			Provenance: envelope, Value: operation.Value,
			Digest: kernel.CanonicalDigest(operation.Value),
		}
		track(key, operation.Address.ObjectID)
	}
	byObject := map[ObjectID][]Unit{}
	for _, unit := range units {
		final = append(final, unit)
		byObject[unit.ObjectID] = append(byObject[unit.ObjectID], unit)
	}
	for _, siblings := range byObject {
		if _, err := AssembleUnits(siblings); err != nil {
			return nil, nil, err
		}
	}
	sort.Slice(final, func(i, j int) bool {
		return AddressKey(final[i].Address) < AddressKey(final[j].Address)
	})
	sort.Slice(deleted, func(i, j int) bool {
		return AddressKey(deleted[i]) < AddressKey(deleted[j])
	})
	return final, deleted, nil
}

func AssembleObjectValue(repository kernel.RepositoryID, objectID ObjectID, commit kernel.CommitID, units []Unit) (ObjectValue, error) {
	assembled, err := AssembleUnits(units)
	if err != nil {
		return ObjectValue{}, err
	}
	value := ObjectValue{
		ObjectRef:  ObjectRef{Repository: repository, Object: objectID},
		Repository: repository, Commit: commit,
		Address: Address{Kind: KindEntity, ObjectID: objectID},
		Value:   assembled, Declarations: DeclareUnits(units),
	}
	if len(units) == 1 {
		value.Provenance = units[0].Provenance
	}
	for _, unit := range units {
		if unit.Address.AspectName == "" {
			continue
		}
		for _, member := range units {
			value.Units = append(value.Units, member.Address)
		}
		break
	}
	return value, nil
}

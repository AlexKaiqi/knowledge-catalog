// Package unitcodec owns provider-neutral unit mutation and assembly. File
// repositories add frontmatter/path serialization around it; native providers
// persist the same units as typed rows.
package unitcodec

import (
	"sort"

	"kc/kernel"
	"kc/knowledge"
)

// Unit is the provider-neutral knowledge atom. It deliberately has no storage
// path, filename, extension, or serialized-file field.
type Unit struct {
	ObjectID    knowledge.ObjectID
	Address     knowledge.Address
	PathHint    string
	SchemaRef   string
	ValueSource *knowledge.ValueSource
	Provenance  *knowledge.ProvenanceEnvelope
	Value       any
	Digest      kernel.Digest
}

func Assemble(units []Unit) (any, error) {
	if len(units) == 0 {
		return nil, nil
	}
	var blobs, parts []Unit
	for _, unit := range units {
		if knowledge.IsEntityBlob(unit.Address) {
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

func Declarations(units []Unit) []knowledge.UnitDeclaration {
	sorted := append([]Unit(nil), units...)
	sort.Slice(sorted, func(i, j int) bool {
		return knowledge.AddressKey(sorted[i].Address) < knowledge.AddressKey(sorted[j].Address)
	})
	out := make([]knowledge.UnitDeclaration, 0, len(sorted))
	for _, unit := range sorted {
		out = append(out, knowledge.UnitDeclaration{
			Address: unit.Address, Digest: unit.Digest,
			DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource,
		})
	}
	return out
}

func DeclarationDigest(units []Unit) kernel.Digest {
	declarations := Declarations(units)
	rows := make([]any, 0, len(declarations))
	for _, declaration := range declarations {
		rows = append(rows, map[string]any{
			"address": knowledge.AddressKey(declaration.Address),
			"digest":  declaration.DeclarationDigest,
		})
	}
	return kernel.CanonicalDigest(rows)
}

// Apply mutates only the supplied object working set. Callers load the units
// for object IDs touched by operations; no repository-wide tree is required.
func Apply(existing []Unit, operations []knowledge.Operation, provenance *knowledge.ProvenanceEnvelope) (final []Unit, deleted []knowledge.Address, err error) {
	units := map[string]Unit{}
	for _, unit := range existing {
		units[knowledge.AddressKey(unit.Address)] = unit
	}
	for _, operation := range operations {
		if err := knowledge.AssertWritable(operation.Address); err != nil {
			return nil, nil, err
		}
		key := knowledge.AddressKey(operation.Address)
		current, exists := units[key]
		siblings := make([]Unit, 0)
		for _, unit := range units {
			if unit.ObjectID == operation.Address.ObjectID {
				siblings = append(siblings, unit)
			}
		}
		if operation.Op == knowledge.OpRemove {
			if knowledge.IsEntityBlob(operation.Address) {
				if len(siblings) == 0 {
					return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
						"object %s does not exist", operation.Address.ObjectID)
				}
				if operation.Precondition != nil && operation.Precondition.Digest != "" {
					assembled, err := Assemble(siblings)
					if err != nil {
						return nil, nil, err
					}
					if kernel.CanonicalDigest(assembled) != operation.Precondition.Digest {
						return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
							"digest mismatch for %s", operation.Address.ObjectID)
					}
				}
				for siblingKey, sibling := range units {
					if sibling.ObjectID == operation.Address.ObjectID {
						deleted = append(deleted, sibling.Address)
						delete(units, siblingKey)
					}
				}
				continue
			}
			if !exists {
				return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
					"address %s does not exist", key)
			}
			if operation.Precondition != nil && operation.Precondition.Digest != "" &&
				operation.Precondition.Digest != current.Digest {
				return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
					"digest mismatch for %s", key)
			}
			deleted = append(deleted, current.Address)
			delete(units, key)
			continue
		}
		if err := assertLayout(siblings, operation.Address); err != nil {
			return nil, nil, err
		}
		if operation.Precondition != nil {
			switch operation.Precondition.Type {
			case knowledge.IfAbsent:
				if exists {
					return nil, nil, kernel.Fail(kernel.ErrPreconditionFailed,
						"address %s already exists", key)
				}
			case knowledge.IfObjectEquals, knowledge.IfDigestEquals:
				if operation.Precondition.Digest != "" &&
					(!exists || current.Digest != operation.Precondition.Digest) {
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
		source := operation.ValueSource
		if source == nil && exists {
			source = current.ValueSource
		} else {
			source = source.Normalized()
		}
		envelope := provenance
		if envelope == nil && exists {
			envelope = current.Provenance
		}
		units[key] = Unit{
			ObjectID: operation.Address.ObjectID, Address: operation.Address,
			PathHint: pathHint, SchemaRef: schemaRef, ValueSource: source,
			Provenance: envelope, Value: operation.Value,
			Digest: kernel.CanonicalDigest(operation.Value),
		}
	}
	for _, unit := range units {
		final = append(final, unit)
	}
	sort.Slice(final, func(i, j int) bool {
		return knowledge.AddressKey(final[i].Address) < knowledge.AddressKey(final[j].Address)
	})
	sort.Slice(deleted, func(i, j int) bool {
		return knowledge.AddressKey(deleted[i]) < knowledge.AddressKey(deleted[j])
	})
	return final, deleted, nil
}

func assertLayout(units []Unit, incoming knowledge.Address) error {
	hasBlob, hasAspect := false, false
	for _, unit := range units {
		if knowledge.IsEntityBlob(unit.Address) {
			hasBlob = true
		} else {
			hasAspect = true
		}
	}
	if hasBlob && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", incoming.ObjectID)
	}
	if knowledge.IsEntityBlob(incoming) && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict,
			"cannot PUT an entity blob on %s; object already has aspects", incoming.ObjectID)
	}
	if !knowledge.IsEntityBlob(incoming) && hasBlob {
		return kernel.Fail(kernel.ErrObjectIDConflict,
			"cannot PUT an aspect on %s; object is an entity blob", incoming.ObjectID)
	}
	return nil
}

func AssembleKnowledgeValue(repository kernel.RepositoryID, objectID knowledge.ObjectID, commit kernel.CommitID, units []Unit) (knowledge.KnowledgeValue, error) {
	assembled, err := Assemble(units)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repository, Object: objectID},
		Repository:   repository, Commit: commit,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
		Value:   assembled, Declarations: Declarations(units),
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

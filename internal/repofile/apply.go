package repofile

import (
	"kc/kernel"
	"kc/knowledge"
)

// Apply mutates the tree for one PUT/REMOVE. toWrite/toDelete collect path contents.
func Apply(idx *Tree, op knowledge.Operation, prov *knowledge.ProvenanceEnvelope, toWrite map[string]string, toDelete map[string]struct{}) error {
	if err := knowledge.AssertWritable(op.Address); err != nil {
		return err
	}
	if op.Op == knowledge.OpPut {
		return applyPut(idx, op, prov, toWrite, toDelete)
	}
	return applyRemove(idx, op, toWrite, toDelete)
}

func applyPut(idx *Tree, op knowledge.Operation, prov *knowledge.ProvenanceEnvelope, toWrite map[string]string, toDelete map[string]struct{}) error {
	siblings := idx.ObjectUnits(op.Address.ObjectID)
	existing, has := idx.Units[knowledge.AddressKey(op.Address)]
	if err := AssertLayout(siblings, op.Address); err != nil {
		return err
	}
	if op.Precondition != nil {
		if op.Precondition.Type == knowledge.IfAbsent && has {
			return kernel.Fail(kernel.ErrPreconditionFailed, "address %s already exists", knowledge.AddressKey(op.Address))
		}
		if (op.Precondition.Type == knowledge.IfObjectEquals || op.Precondition.Type == knowledge.IfDigestEquals) && op.Precondition.Digest != "" {
			if !has || existing.Digest != op.Precondition.Digest {
				actual := "missing"
				if has {
					actual = string(existing.Digest)
				}
				return kernel.Fail(kernel.ErrPreconditionFailed, "digest mismatch for %s: expected %s, actual %s", knowledge.AddressKey(op.Address), op.Precondition.Digest, actual)
			}
		}
	}
	pathHint := op.PathHint
	if pathHint == "" && has {
		pathHint = existing.Path
	}
	if pathHint == "" {
		pathHint = DefaultPath(op.Address)
	}
	newPath, err := SafeRelativePath(pathHint)
	if err != nil {
		return err
	}
	if !KnowledgePath(newPath) {
		return kernel.Fail(kernel.ErrUsageInvalid, "path must use a readable knowledge file extension (.json, .md, .yaml, .yml, .txt): %s", newPath)
	}
	if has && existing.Path != newPath {
		toDelete[existing.Path] = struct{}{}
		delete(toWrite, existing.Path)
	}
	storedHint := existing.PathHint
	if op.PathHint != "" {
		storedHint = newPath
	}
	schema := op.SchemaRef
	if schema == "" && has {
		schema = existing.SchemaRef
	}
	source := op.ValueSource
	if source == nil && has {
		source = existing.ValueSource
	} else {
		source = source.Normalized()
	}
	envelope := prov
	if envelope == nil && has {
		envelope = existing.Provenance
	}
	unit := Unit{
		ObjectID: op.Address.ObjectID, Address: op.Address, PathHint: storedHint,
		SchemaRef: schema, ValueSource: source, Provenance: envelope, Value: op.Value,
		Path: newPath, Digest: kernel.CanonicalDigest(op.Value),
	}
	content, err := SerializeWithSource(op.Address, unit.PathHint, unit.SchemaRef, unit.ValueSource, unit.Provenance, op.Value)
	if err != nil {
		return err
	}
	toWrite[newPath] = content
	idx.Upsert(unit)
	return nil
}

func applyRemove(idx *Tree, op knowledge.Operation, toWrite map[string]string, toDelete map[string]struct{}) error {
	if knowledge.IsEntityBlob(op.Address) {
		units := idx.ObjectUnits(op.Address.ObjectID)
		if len(units) == 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "object %s does not exist", op.Address.ObjectID)
		}
		if op.Precondition != nil && (op.Precondition.Type == knowledge.IfObjectEquals || op.Precondition.Type == knowledge.IfDigestEquals) && op.Precondition.Digest != "" {
			assembled, err := Assemble(units)
			if err != nil {
				return err
			}
			if kernel.CanonicalDigest(assembled) != op.Precondition.Digest {
				return kernel.Fail(kernel.ErrPreconditionFailed, "digest mismatch for %s: expected %s, actual %s", op.Address.ObjectID, op.Precondition.Digest, kernel.CanonicalDigest(assembled))
			}
		}
		for _, unit := range units {
			toDelete[unit.Path] = struct{}{}
			delete(toWrite, unit.Path)
			idx.Remove(unit.Address)
		}
		return nil
	}

	existing, has := idx.Units[knowledge.AddressKey(op.Address)]
	if !has {
		return kernel.Fail(kernel.ErrPreconditionFailed, "address %s does not exist", knowledge.AddressKey(op.Address))
	}
	if op.Precondition != nil && (op.Precondition.Type == knowledge.IfObjectEquals || op.Precondition.Type == knowledge.IfDigestEquals) && op.Precondition.Digest != existing.Digest {
		return kernel.Fail(kernel.ErrPreconditionFailed, "digest mismatch for %s: expected %s, actual %s", knowledge.AddressKey(op.Address), op.Precondition.Digest, existing.Digest)
	}
	toDelete[existing.Path] = struct{}{}
	delete(toWrite, existing.Path)
	idx.Remove(op.Address)
	return nil
}

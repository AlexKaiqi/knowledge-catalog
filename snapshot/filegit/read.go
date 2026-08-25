package filegit

import (
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
)

func (r *FileGitRepository) Resolve(objectID knowledge.ObjectID, commitID kernel.CommitID) (knowledge.Resolution, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) > 0 {
		assembled, err := repofile.Assemble(units)
		if err != nil {
			return knowledge.Resolution{}, err
		}
		schema := ""
		if len(units) == 1 {
			schema = units[0].SchemaRef
		}
		return knowledge.Resolution{
			Repository:        r.repositoryID,
			Commit:            commitID,
			ObjectID:          objectID,
			Address:           knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
			PathHint:          repofile.EntityPathHint(units, objectID),
			Digest:            kernel.CanonicalDigest(assembled),
			DeclarationDigest: repofile.TreeDeclarationDigest(units),
			SchemaRef:         schema,
			ValueSource: func() *knowledge.ValueSource {
				if len(units) == 1 {
					return units[0].ValueSource
				}
				return nil
			}(),
			Status: knowledge.StatusResolved,
		}, nil
	}
	status := knowledge.StatusUnresolved
	if r.everExisted(objectID) {
		status = knowledge.StatusRemoved
	}
	return knowledge.Resolution{
		Repository: r.repositoryID,
		Commit:     commitID,
		ObjectID:   objectID,
		Address:    knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
		Status:     status,
	}, nil
}

func (r *FileGitRepository) Read(objectID knowledge.ObjectID, commitID kernel.CommitID) (knowledge.KnowledgeValue, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commitID)
	}
	return r.assembleValue(objectID, commitID, units)
}

// assembleValue hydrates one object from units that were already read from the
// same pinned tree. Callers that enumerate a tree must reuse that scan instead
// of calling Read for every object and turning a linear list into N full scans.
func (r *FileGitRepository) assembleValue(objectID knowledge.ObjectID, commitID kernel.CommitID, units []repofile.Unit) (knowledge.KnowledgeValue, error) {
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	kv := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.repositoryID, Object: objectID},
		Repository:   r.repositoryID,
		Commit:       commitID,
		Address:      knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
		Value:        assembled,
		Declarations: repofile.Declarations(units),
	}
	if len(units) == 1 {
		kv.Provenance = units[0].Provenance
	}
	for _, u := range units {
		if u.Address.AspectName != "" {
			for _, unit := range units {
				kv.Units = append(kv.Units, unit.Address)
			}
			break
		}
	}
	return kv, nil
}

func (r *FileGitRepository) ResolveAddress(address knowledge.Address, commitID kernel.CommitID) (knowledge.Resolution, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.Resolution{}, err
	}
	idx, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if unit, ok := idx.Units[knowledge.AddressKey(address)]; ok {
		hint := unit.PathHint
		if hint == "" {
			hint = unit.Path
		}
		return knowledge.Resolution{
			Repository: r.repositoryID, Commit: commitID, ObjectID: address.ObjectID,
			Address: unit.Address, PathHint: hint, Digest: unit.Digest,
			DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: knowledge.StatusResolved,
		}, nil
	}
	status := knowledge.StatusUnresolved
	if r.everExisted(address.ObjectID) {
		status = knowledge.StatusRemoved
	}
	return knowledge.Resolution{Repository: r.repositoryID, Commit: commitID, ObjectID: address.ObjectID, Address: address, Status: status}, nil
}

func (r *FileGitRepository) ReadAddress(address knowledge.Address, commitID kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	idx, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	unit, ok := idx.Units[knowledge.AddressKey(address)]
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", knowledge.AddressKey(address), commitID)
	}
	return knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.repositoryID, Object: address.ObjectID},
		Repository:   r.repositoryID,
		Commit:       commitID,
		Address:      unit.Address,
		Value:        unit.Value,
		Provenance:   unit.Provenance,
		Declarations: []knowledge.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *FileGitRepository) GetProvenance(objectID knowledge.ObjectID, commitID kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	units := idx.ObjectUnits(objectID)
	if len(units) == 0 {
		return knowledge.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commitID)
	}
	sorted := append([]repofile.Unit{}, units...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if knowledge.AddressKey(sorted[j].Address) < knowledge.AddressKey(sorted[i].Address) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	chain := []knowledge.ProvenanceEnvelope{}
	for _, u := range sorted {
		if u.Provenance != nil {
			chain = append(chain, *u.Provenance)
		}
	}
	return knowledge.ProvenanceTrace{Repository: r.repositoryID, Commit: commitID, ObjectID: objectID, Chain: chain}, nil
}

func (r *FileGitRepository) List(commitID kernel.CommitID) ([]knowledge.KnowledgeValue, error) {
	idx, err := r.scanAt(commitID)
	if err != nil {
		return nil, err
	}
	var out []knowledge.KnowledgeValue
	for objectID, units := range idx.ByObject {
		value, err := r.assembleValue(objectID, commitID, units)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

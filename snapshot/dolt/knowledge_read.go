package dolt

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
)

func (r *DoltRepository) Resolve(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.Resolution, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.resolveLocked(objectID, commit)
}

func (r *DoltRepository) resolveLocked(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.Resolution, error) {
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	units := tree.ObjectUnits(objectID)
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
			Repository: r.repositoryID, Commit: commit, ObjectID: objectID,
			Address:  knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
			PathHint: repofile.EntityPathHint(units, objectID), Digest: kernel.CanonicalDigest(assembled),
			DeclarationDigest: repofile.TreeDeclarationDigest(units),
			SchemaRef:         schema, ValueSource: func() *knowledge.ValueSource {
				if len(units) == 1 {
					return units[0].ValueSource
				}
				return nil
			}(), Status: knowledge.StatusResolved,
		}, nil
	}
	status := knowledge.StatusUnresolved
	if r.everExistedLocked(objectID) {
		status = knowledge.StatusRemoved
	}
	return knowledge.Resolution{
		Repository: r.repositoryID, Commit: commit, ObjectID: objectID,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}, Status: status,
	}, nil
}

func (r *DoltRepository) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.readLocked(objectID, commit)
}

func (r *DoltRepository) readLocked(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	units := tree.ObjectUnits(objectID)
	if len(units) == 0 {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	assembled, err := repofile.Assemble(units)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.repositoryID, Object: objectID},
		Repository:   r.repositoryID, Commit: commit,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID}, Value: assembled,
		Declarations: repofile.Declarations(units),
	}
	if len(units) == 1 {
		value.Provenance = units[0].Provenance
	}
	for _, unit := range units {
		if unit.Address.AspectName != "" {
			for _, member := range units {
				value.Units = append(value.Units, member.Address)
			}
			break
		}
	}
	return value, nil
}

func (r *DoltRepository) ResolveAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.Resolution, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.Resolution{}, err
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	if unit, ok := tree.Units[knowledge.AddressKey(address)]; ok {
		hint := unit.PathHint
		if hint == "" {
			hint = unit.Path
		}
		return knowledge.Resolution{
			Repository: r.repositoryID, Commit: commit, ObjectID: address.ObjectID,
			Address: unit.Address, PathHint: hint, Digest: unit.Digest,
			DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
			SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource, Status: knowledge.StatusResolved,
		}, nil
	}
	status := knowledge.StatusUnresolved
	if r.everExistedLocked(address.ObjectID) {
		status = knowledge.StatusRemoved
	}
	return knowledge.Resolution{Repository: r.repositoryID, Commit: commit, ObjectID: address.ObjectID, Address: address, Status: status}, nil
}

func (r *DoltRepository) ReadAddress(address knowledge.Address, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	unit, ok := tree.Units[knowledge.AddressKey(address)]
	if !ok {
		return knowledge.KnowledgeValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address %s is missing at commit %s", knowledge.AddressKey(address), commit)
	}
	return knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: r.repositoryID, Object: address.ObjectID},
		Repository:   r.repositoryID, Commit: commit, Address: unit.Address,
		Value: unit.Value, Provenance: unit.Provenance,
		Declarations: []knowledge.UnitDeclaration{repofile.DeclarationOf(unit)},
	}, nil
}

func (r *DoltRepository) GetProvenance(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	units := append([]repofile.Unit(nil), tree.ObjectUnits(objectID)...)
	if len(units) == 0 {
		return knowledge.ProvenanceTrace{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object %s is missing at commit %s", objectID, commit)
	}
	sort.Slice(units, func(i, j int) bool {
		return knowledge.AddressKey(units[i].Address) < knowledge.AddressKey(units[j].Address)
	})
	chain := []knowledge.ProvenanceEnvelope{}
	for _, unit := range units {
		if unit.Provenance != nil {
			chain = append(chain, *unit.Provenance)
		}
	}
	return knowledge.ProvenanceTrace{Repository: r.repositoryID, Commit: commit, ObjectID: objectID, Chain: chain}, nil
}

func (r *DoltRepository) List(commit kernel.CommitID) ([]knowledge.KnowledgeValue, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	tree, err := r.scanAtLocked(commit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(tree.ByObject))
	for objectID := range tree.ByObject {
		ids = append(ids, string(objectID))
	}
	sort.Strings(ids)
	out := make([]knowledge.KnowledgeValue, 0, len(ids))
	for _, id := range ids {
		value, err := r.readLocked(knowledge.ObjectID(id), commit)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

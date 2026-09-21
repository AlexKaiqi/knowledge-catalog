package reader

import (
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
)

func (r *treeRepository) addressAtPaths(address knowledge.Address, commit kernel.CommitID, paths []string) (repofile.Unit, error) {
	if err := knowledge.AssertWritable(address); err != nil {
		return repofile.Unit{}, err
	}
	units, err := readObjectUnitsAtPaths(r.tree, paths, address.ObjectID, commit)
	if err != nil {
		return repofile.Unit{}, err
	}
	for _, unit := range units {
		if knowledge.AddressKey(unit.Address) == knowledge.AddressKey(address) {
			return unit, nil
		}
	}
	return repofile.Unit{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address is outside selected file paths")
}

func (r *treeRepository) ReadAddressAtPaths(address knowledge.Address, commit kernel.CommitID, paths []string) (knowledge.KnowledgeValue, error) {
	unit, err := r.addressAtPaths(address, commit, paths)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	return knowledge.KnowledgeValue{KnowledgeRef: knowledge.KnowledgeRef{Repository: r.ID(), Object: address.ObjectID}, Repository: r.ID(), Commit: commit, Address: unit.Address, Value: unit.Value, Provenance: unit.Provenance, Declarations: []knowledge.UnitDeclaration{repofile.DeclarationOf(unit)}}, nil
}

func (r *treeRepository) ResolveAddressAtPaths(address knowledge.Address, commit kernel.CommitID, paths []string) (knowledge.Resolution, error) {
	unit, err := r.addressAtPaths(address, commit, paths)
	if err != nil {
		return knowledge.Resolution{}, err
	}
	hint := unit.PathHint
	if hint == "" {
		hint = unit.Path
	}
	return knowledge.Resolution{Repository: r.ID(), Commit: commit, ObjectID: address.ObjectID, Address: unit.Address, PathHint: hint, Digest: unit.Digest, DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource), SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource, Status: knowledge.StatusResolved}, nil
}

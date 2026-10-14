package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

// A direct Git publication may declare the protocol README at its one bounded
// path without Writer locators. This preserves that existing read contract;
// ordinary units still require their fixed maintenance coordinates.
func (r *treeRepository) readmeAddressUnit(address objectmodel.Address, commit kernel.CommitID, paths []string) (repofile.Unit, bool, error) {
	if !objectmodel.IsReadmeAddress(address, string(objectmodel.CoreReadmeSchemaV1)) ||
		paths != nil && !containsPath(paths, objectmodel.RepositoryReadmePath) {
		return repofile.Unit{}, false, nil
	}
	content, err := r.tree.ReadFile(objectmodel.RepositoryReadmePath, commit)
	if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
		return repofile.Unit{}, false, nil
	}
	if err != nil {
		return repofile.Unit{}, false, err
	}
	unit := repofile.Parse(string(content))
	if unit == nil || !objectmodel.IsReadmeAddress(unit.Address, unit.SchemaRef) ||
		objectmodel.AddressKey(unit.Address) != objectmodel.AddressKey(address) {
		return repofile.Unit{}, false, nil
	}
	tree := repofile.NewTree()
	if err := repofile.Ingest(tree, unit, objectmodel.RepositoryReadmePath); err != nil {
		return repofile.Unit{}, false, err
	}
	return tree.ObjectUnits(address.ObjectID)[0], true, nil
}

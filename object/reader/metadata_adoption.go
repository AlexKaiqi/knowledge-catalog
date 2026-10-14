package reader

import (
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

func (r *treeRepository) AdoptedSchemaRefs(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	locations, err := repofile.ReadObjectUnitLocations(r.tree, id, at)
	if err != nil {
		return nil, err
	}
	if len(locations) == 0 {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "object has no published maintenance coordinates")
	}
	seen := map[string]bool{}
	refs := []string{}
	for _, location := range locations {
		if location.SchemaRef == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object locator lacks adopted Schema references; explicitly rebuild locators")
		}
		ref := *location.SchemaRef
		if ref != "" && !seen[ref] {
			refs = append(refs, ref)
			seen[ref] = true
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func (r *nativeReadRepository) AdoptedSchemaRefs(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	port, err := nativeReadPort[objectmodel.SchemaAdoptionLocator](r.readRepository, "Schema adoption locator")
	if err != nil {
		return nil, err
	}
	return port.AdoptedSchemaRefs(id, at)
}

func (r *datasetRepository) AdoptedSchemaRefs(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	if err := r.check(id, at); err != nil {
		return nil, err
	}
	port, err := nativeReadPort[objectmodel.SchemaAdoptionLocator](r.Repository, "Schema adoption locator")
	if err != nil {
		return nil, err
	}
	return port.AdoptedSchemaRefs(id, at)
}

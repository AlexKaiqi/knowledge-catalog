package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

// classifyMemberSources uses only Writer's fixed namespace coordinates.
// It identifies a declared Binding without reading its Schema or interpreting
// origin, runtime, protocol or operations. Those remain capability work.
func classifyMemberSources(store snapshot.TreeReader, repository kernel.RepositoryID, commit kernel.CommitID, units []repofile.Unit) ([]repofile.Unit, error) {
	classified := append([]repofile.Unit(nil), units...)
	coordinates := map[kernel.CommitID]map[objectmodel.ObjectID]bool{}
	for i := range classified {
		unit := &classified[i]
		ref, needed, err := memberSourceCoordinate(repository, unit.Address, unit.SchemaRef, unit.ValueSource)
		if err != nil {
			return nil, err
		}
		if !needed {
			continue
		}
		at := ref.Commit
		if at == "" {
			at = commit
		}
		bound, present := coordinates[at]
		if !present {
			ids, err := (&treeManifestLocator{tree: store}).BindingSchemaObjectIDs(at)
			if err != nil {
				return nil, err
			}
			bound = memberBindingSet(ids)
			coordinates[at] = bound
		}
		if bound[ref.Object] {
			unit.ValueSource = memberBindingMarker()
		}
	}
	return classified, nil
}

func memberSourceCoordinate(repository kernel.RepositoryID, address objectmodel.Address, schemaRef string, source *objectmodel.ValueSource) (objectmodel.ParsedSchemaRef, bool, error) {
	if address.Kind != objectmodel.KindMember || schemaRef == "" || source.Normalized() != nil {
		return objectmodel.ParsedSchemaRef{}, false, nil
	}
	ref, valid := objectmodel.ParseSchemaRef(schemaRef)
	if !valid || ref.Repository != "" && ref.Repository != repository {
		return objectmodel.ParsedSchemaRef{}, false, kernel.Fail(kernel.ErrSchemaRevisionUnresolved, "Member has no valid fixed Schema coordinate")
	}
	return ref, true, nil
}

func memberBindingSet(ids []objectmodel.ObjectID) map[objectmodel.ObjectID]bool {
	bound := make(map[objectmodel.ObjectID]bool, len(ids))
	for _, id := range ids {
		bound[id] = true
	}
	return bound
}

func memberBindingMarker() *objectmodel.ValueSource {
	// The namespace proves only the source category. Its mode and executable
	// declaration remain unresolved until the capability reads the fixed Schema.
	return &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding}
}

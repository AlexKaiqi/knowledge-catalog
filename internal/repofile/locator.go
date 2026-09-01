package repofile

import (
	"encoding/json"
	"sort"

	"kc/knowledge"
)

// LocatorManifestPath is layer ② exact-read metadata for tree-backed
// authorities. It maps object identities to bounded unit paths; it contains no
// relation endpoints, predicates, or searchable body fields.
const LocatorManifestPath = ".kc/knowledge-units.index"

type LocatorManifest struct {
	Objects        map[knowledge.ObjectID][]string `json:"objects"`
	Schemas        []knowledge.ObjectID            `json:"schemas,omitempty"`
	BindingSchemas []knowledge.ObjectID            `json:"bindingSchemas,omitempty"`
	// Referrers maps a schema identity to the exact unit Addresses that declare
	// it. It is bounded reverse-dependency metadata for Schema publication, not
	// a searchable field index.
	Referrers map[knowledge.ObjectID][]knowledge.Address `json:"referrers,omitempty"`
}

func BuildLocatorManifest(tree *Tree) LocatorManifest {
	manifest := LocatorManifest{Objects: map[knowledge.ObjectID][]string{}}
	bindingSchemas := map[knowledge.ObjectID]struct{}{}
	referrers := map[knowledge.ObjectID][]knowledge.Address{}
	for objectID, units := range tree.ByObject {
		paths := make([]string, 0, len(units))
		for _, unit := range units {
			paths = append(paths, unit.Path)
			parsed, ok := knowledge.ParseSchemaRef(unit.SchemaRef)
			if !ok {
				continue
			}
			if unit.ValueSource != nil && unit.ValueSource.Kind == knowledge.ValueSourceBinding {
				bindingSchemas[parsed.Object] = struct{}{}
			}
			if !knowledge.IsSchemaObject(objectID) {
				referrers[parsed.Object] = append(referrers[parsed.Object], unit.Address)
			}
		}
		sort.Strings(paths)
		manifest.Objects[objectID] = paths
		if knowledge.IsSchemaObject(objectID) {
			manifest.Schemas = append(manifest.Schemas, objectID)
		}
	}
	for objectID := range bindingSchemas {
		manifest.BindingSchemas = append(manifest.BindingSchemas, objectID)
	}
	if len(referrers) > 0 {
		for schema := range referrers {
			addresses := referrers[schema]
			sort.Slice(addresses, func(i, j int) bool {
				return knowledge.AddressKey(addresses[i]) < knowledge.AddressKey(addresses[j])
			})
		}
		manifest.Referrers = referrers
	}
	sort.Slice(manifest.Schemas, func(i, j int) bool { return manifest.Schemas[i] < manifest.Schemas[j] })
	sort.Slice(manifest.BindingSchemas, func(i, j int) bool { return manifest.BindingSchemas[i] < manifest.BindingSchemas[j] })
	return manifest
}

func EncodeLocatorManifest(manifest LocatorManifest) ([]byte, error) {
	return json.Marshal(manifest)
}

func DecodeLocatorManifest(raw []byte) (LocatorManifest, error) {
	var manifest LocatorManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return LocatorManifest{}, err
	}
	if manifest.Objects == nil {
		manifest.Objects = map[knowledge.ObjectID][]string{}
	}
	return manifest, nil
}

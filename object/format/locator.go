package format

import (
	"encoding/json"
	"path"
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// LocatorManifestPath is layer ② exact-read metadata for tree-backed
// authorities. It maps object identities to bounded unit paths; it contains no
// relation endpoints, predicates, or searchable body fields.
const LocatorManifestPath = ".kc/object-units.index"

// LocatorObjectDirectory contains one small entry per object. The filename is
// a canonical digest rather than the object identity, so identities never
// become paths and an exact lookup reads one bounded blob.
const LocatorObjectDirectory = ".kc/object-locators/objects"
const LocatorCompletePath = ".kc/object-locators/complete"
const LocatorSchemaIndexPath = ".kc/object-locators/schemas.index"
const LocatorBindingIndexPath = ".kc/object-locators/bindings.index"
const LocatorCompleteBody = "v2\n"

type ObjectLocatorEntry struct {
	ObjectID objectmodel.ObjectID `json:"objectId"`
	Paths    []string             `json:"paths"`
	Units    []ObjectUnitLocation `json:"units,omitempty"`
}

func EncodeSchemaIndex(ids []objectmodel.ObjectID) ([]byte, error) {
	ids = append([]objectmodel.ObjectID(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return json.Marshal(ids)
}

func DecodeSchemaIndex(raw []byte) ([]objectmodel.ObjectID, error) {
	ids := []objectmodel.ObjectID{}
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !objectmodel.IsSchemaObject(id) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "schema locator contains non-schema object %s", id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func ObjectLocatorPath(objectID objectmodel.ObjectID) string {
	name := kernel.CanonicalDigest(map[string]any{"objectId": objectID})
	return path.Join(LocatorObjectDirectory, string(name)+".entry")
}

func EncodeObjectLocator(objectID objectmodel.ObjectID, paths []string) ([]byte, error) {
	entry := ObjectLocatorEntry{ObjectID: objectID, Paths: append([]string(nil), paths...)}
	sort.Strings(entry.Paths)
	return json.Marshal(entry)
}

func DecodeObjectLocator(raw []byte, expected objectmodel.ObjectID) ([]string, error) {
	entry, err := decodeExpectedObjectLocator(raw, expected)
	if err != nil {
		return nil, err
	}
	return entry.Paths, nil
}

func decodeExpectedObjectLocator(raw []byte, expected objectmodel.ObjectID) (ObjectLocatorEntry, error) {
	entry, err := DecodeObjectLocatorEntry(raw)
	if err != nil {
		return ObjectLocatorEntry{}, err
	}
	if entry.ObjectID != expected {
		return ObjectLocatorEntry{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"object locator contains %s, expected %s", entry.ObjectID, expected)
	}
	return entry, nil
}

func DecodeObjectLocatorEntry(raw []byte) (ObjectLocatorEntry, error) {
	var entry ObjectLocatorEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ObjectLocatorEntry{}, err
	}
	if entry.ObjectID == "" {
		return ObjectLocatorEntry{}, kernel.Fail(kernel.ErrPreconditionFailed, "object locator has no objectId")
	}
	entry.Paths = append([]string(nil), entry.Paths...)
	if entry.Units != nil {
		if err := validateObjectUnitLocations(entry); err != nil {
			return ObjectLocatorEntry{}, err
		}
		entry.Units = append([]ObjectUnitLocation(nil), entry.Units...)
	}
	return entry, nil
}

// ReadObjectLocator prefers the per-object format. The legacy whole-repository
// manifest is read only for an authority not yet migrated by a Writer commit.
func ReadObjectLocator(tree snapshot.TreeReader, objectID objectmodel.ObjectID, commit kernel.CommitID) ([]string, error) {
	raw, err := tree.ReadFile(ObjectLocatorPath(objectID), commit)
	if err == nil {
		paths, decodeErr := DecodeObjectLocator(raw, objectID)
		if decodeErr != nil {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed,
				"invalid object locator for %s at %s: %v", objectID, commit, decodeErr)
		}
		return paths, nil
	}
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return nil, err
	}
	if _, completeErr := tree.ReadFile(LocatorCompletePath, commit); completeErr == nil {
		return nil, nil
	} else if kernel.CodeOf(completeErr) != kernel.ErrObjectRefUnresolved {
		return nil, completeErr
	}
	raw, err = tree.ReadFile(LocatorManifestPath, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	manifest, err := DecodeLocatorManifest(raw)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed,
			"invalid legacy object locator at %s: %v", commit, err)
	}
	return append([]string(nil), manifest.Objects[objectID]...), nil
}

type LocatorManifest struct {
	Objects        map[objectmodel.ObjectID][]string `json:"objects"`
	Schemas        []objectmodel.ObjectID            `json:"schemas,omitempty"`
	BindingSchemas []objectmodel.ObjectID            `json:"bindingSchemas,omitempty"`
	// Referrers maps a schema identity to the exact unit Addresses that declare
	// it. It is bounded reverse-dependency metadata for Schema publication, not
	// a searchable field index.
	Referrers map[objectmodel.ObjectID][]objectmodel.Address `json:"referrers,omitempty"`
}

func BuildLocatorManifest(tree *Tree) LocatorManifest {
	manifest := LocatorManifest{Objects: map[objectmodel.ObjectID][]string{}}
	bindingSchemas := map[objectmodel.ObjectID]struct{}{}
	referrers := map[objectmodel.ObjectID][]objectmodel.Address{}
	for objectID, units := range tree.ByObject {
		paths := make([]string, 0, len(units))
		for _, unit := range units {
			paths = append(paths, unit.Path)
			if objectmodel.IsSchemaObject(objectID) {
				if definition, err := objectmodel.ParseSchemaDefinition(objectID, unit.Value); err == nil && definition.Bound() {
					bindingSchemas[objectID] = struct{}{}
				}
			}
			parsed, ok := objectmodel.ParseSchemaRef(unit.SchemaRef)
			if !ok {
				continue
			}
			if unit.ValueSource != nil && unit.ValueSource.Kind == objectmodel.ValueSourceBinding {
				bindingSchemas[parsed.Object] = struct{}{}
			}
			if !objectmodel.IsSchemaObject(objectID) {
				referrers[parsed.Object] = append(referrers[parsed.Object], unit.Address)
			}
		}
		sort.Strings(paths)
		manifest.Objects[objectID] = paths
		if objectmodel.IsSchemaObject(objectID) {
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
				return objectmodel.AddressKey(addresses[i]) < objectmodel.AddressKey(addresses[j])
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
		manifest.Objects = map[objectmodel.ObjectID][]string{}
	}
	return manifest, nil
}

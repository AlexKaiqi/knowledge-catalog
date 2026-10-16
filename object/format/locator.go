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

// BindingCoordinate is one binding unit, the Schema it adopts (if any) and
// the ResourceDescriptor it references. The binding index lists every binding
// unit so Writer maintains it exactly from the objects a ChangeSet touches;
// BindingSchemaObjectIDs and BindingReferences are both derived from it.
type BindingCoordinate struct {
	Address    objectmodel.Address  `json:"address"`
	Schema     objectmodel.ObjectID `json:"schema,omitempty"`
	Descriptor objectmodel.ObjectID `json:"descriptor,omitempty"`
}

// BindingCoordinates lists every binding unit in tree.
func BindingCoordinates(tree *Tree) []BindingCoordinate {
	out := []BindingCoordinate{}
	for _, units := range tree.ByObject {
		for _, unit := range units {
			if !unit.ValueSource.IsBinding() {
				continue
			}
			coordinate := BindingCoordinate{Address: unit.Address, Descriptor: unit.ValueSource.Binding.DescriptorRef}
			if parsed, ok := objectmodel.ParseSchemaRef(unit.SchemaRef); ok {
				coordinate.Schema = parsed.Object
			}
			out = append(out, coordinate)
		}
	}
	sortBindingCoordinates(out)
	return out
}

// BindingReferences derives the descriptor references from binding
// coordinates. An index written before coordinates carried descriptors cannot
// answer and requires an explicit locator rebuild instead of a scan.
func BindingReferences(coordinates []BindingCoordinate) ([]objectmodel.BindingReference, error) {
	out := make([]objectmodel.BindingReference, 0, len(coordinates))
	for _, coordinate := range coordinates {
		if coordinate.Descriptor == "" {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"binding unit coordinates predate descriptor references; explicitly rebuild locators")
		}
		out = append(out, objectmodel.BindingReference{Address: coordinate.Address, DescriptorRef: coordinate.Descriptor})
	}
	return out, nil
}

func sortBindingCoordinates(coordinates []BindingCoordinate) {
	sort.Slice(coordinates, func(i, j int) bool {
		return objectmodel.AddressKey(coordinates[i].Address) < objectmodel.AddressKey(coordinates[j].Address)
	})
}

func EncodeBindingIndex(coordinates []BindingCoordinate) ([]byte, error) {
	coordinates = append([]BindingCoordinate{}, coordinates...)
	sortBindingCoordinates(coordinates)
	return json.Marshal(coordinates)
}

func DecodeBindingIndex(raw []byte) ([]BindingCoordinate, error) {
	coordinates := []BindingCoordinate{}
	if err := json.Unmarshal(raw, &coordinates); err != nil {
		return nil, err
	}
	for _, coordinate := range coordinates {
		schemaOK := coordinate.Schema == "" || objectmodel.IsSchemaObject(coordinate.Schema)
		if !schemaOK || coordinate.Address.ObjectID == "" || (coordinate.Schema == "" && coordinate.Descriptor == "") {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "binding index entry %s names neither an adopted schema nor a descriptor", objectmodel.AddressKey(coordinate.Address))
		}
	}
	sortBindingCoordinates(coordinates)
	return coordinates, nil
}

// BindingSchemas returns the distinct Schemas adopted by binding units.
func BindingSchemas(coordinates []BindingCoordinate) []objectmodel.ObjectID {
	seen := map[objectmodel.ObjectID]struct{}{}
	out := []objectmodel.ObjectID{}
	for _, coordinate := range coordinates {
		if coordinate.Schema == "" {
			continue
		}
		if _, dup := seen[coordinate.Schema]; !dup {
			seen[coordinate.Schema] = struct{}{}
			out = append(out, coordinate.Schema)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
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
	Objects  map[objectmodel.ObjectID][]string `json:"objects"`
	Schemas  []objectmodel.ObjectID            `json:"schemas,omitempty"`
	Bindings []BindingCoordinate               `json:"bindings,omitempty"`
	// Referrers maps a schema identity to the exact unit Addresses that declare
	// it. It is bounded reverse-dependency metadata for Schema publication, not
	// a searchable field index.
	Referrers map[objectmodel.ObjectID][]objectmodel.Address `json:"referrers,omitempty"`
}

func BuildLocatorManifest(tree *Tree) LocatorManifest {
	manifest := LocatorManifest{Objects: map[objectmodel.ObjectID][]string{}}
	referrers := map[objectmodel.ObjectID][]objectmodel.Address{}
	for objectID, units := range tree.ByObject {
		paths := make([]string, 0, len(units))
		for _, unit := range units {
			paths = append(paths, unit.Path)
			parsed, ok := objectmodel.ParseSchemaRef(unit.SchemaRef)
			if !ok {
				continue
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
	if bindings := BindingCoordinates(tree); len(bindings) > 0 {
		manifest.Bindings = bindings
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

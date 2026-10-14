package format

import (
	"encoding/json"
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// ObjectUnitLocation is derived exact-read metadata, not object content or
// identity. The immutable locator associates maintenance coordinates with paths.
type ObjectUnitLocation struct {
	Address objectmodel.Address `json:"address"`
	Path    string              `json:"path"`
	// Nil marks metadata written before Schema adoption was retained. A known
	// empty reference differs from missing proof and requires no body lookup.
	SchemaRef *string `json:"schemaRef,omitempty"`
}

func EncodeObjectUnitLocations(objectID objectmodel.ObjectID, locations []ObjectUnitLocation) ([]byte, error) {
	entry := ObjectLocatorEntry{ObjectID: objectID, Units: append([]ObjectUnitLocation(nil), locations...)}
	for _, location := range entry.Units {
		entry.Paths = append(entry.Paths, location.Path)
	}
	if err := validateObjectUnitLocations(entry); err != nil {
		return nil, err
	}
	sort.Strings(entry.Paths)
	sort.Slice(entry.Units, func(i, j int) bool {
		return objectmodel.AddressKey(entry.Units[i].Address) < objectmodel.AddressKey(entry.Units[j].Address)
	})
	return json.Marshal(entry)
}

// ReadObjectUnitLocations never reads object bodies to infer coordinates.
// Paths-only entries remain usable through ReadObjectLocator, but precise
// selection requires coordinates produced by Writer or explicit locator rebuild.
func ReadObjectUnitLocations(tree snapshot.TreeReader, objectID objectmodel.ObjectID, commit kernel.CommitID) ([]ObjectUnitLocation, error) {
	raw, err := tree.ReadFile(ObjectLocatorPath(objectID), commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		paths, err := ReadObjectLocator(tree, objectID, commit)
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return nil, missingUnitCoordinates(objectID)
	}
	if err != nil {
		return nil, err
	}
	entry, err := decodeExpectedObjectLocator(raw, objectID)
	if err != nil {
		return nil, err
	}
	if entry.Units == nil && len(entry.Paths) != 0 {
		return nil, missingUnitCoordinates(objectID)
	}
	return entry.Units, nil
}

func missingUnitCoordinates(objectID objectmodel.ObjectID) error {
	return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object locator for %s has no maintenance coordinates; explicitly rebuild locators before precise selection", objectID)
}

func validateObjectUnitLocations(entry ObjectLocatorEntry) error {
	paths := make(map[string]bool, len(entry.Paths))
	for _, name := range entry.Paths {
		paths[name] = true
	}
	addresses := make(map[string]bool, len(entry.Units))
	for _, location := range entry.Units {
		if location.SchemaRef != nil && *location.SchemaRef != "" {
			if _, ok := objectmodel.ParseSchemaRef(*location.SchemaRef); !ok {
				return kernel.Fail(kernel.ErrPreconditionFailed, "object locator has invalid adopted Schema reference")
			}
		}
		key := objectmodel.AddressKey(location.Address)
		clean, err := SafeRelativePath(location.Path)
		if err != nil || clean != location.Path || !ObjectPath(clean) || !paths[clean] {
			return kernel.Fail(kernel.ErrPreconditionFailed, "object locator has invalid unit path %s", location.Path)
		}
		if location.Address.ObjectID != entry.ObjectID || objectmodel.AssertWritable(location.Address) != nil || addresses[key] {
			return kernel.Fail(kernel.ErrPreconditionFailed, "object locator has invalid or duplicate unit coordinates for %s", entry.ObjectID)
		}
		addresses[key] = true
		delete(paths, clean)
	}
	if len(paths) != 0 || len(entry.Paths) != len(entry.Units) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "object locator unit coordinates do not match paths for %s", entry.ObjectID)
	}
	return nil
}

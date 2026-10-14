package format

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

type metadataOnlyTree struct {
	files map[string][]byte
	reads []string
}

func (tree *metadataOnlyTree) ReadFile(name string, _ kernel.CommitID) ([]byte, error) {
	tree.reads = append(tree.reads, name)
	if raw, ok := tree.files[name]; ok {
		return raw, nil
	}
	return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "file %s is absent", name)
}

func (*metadataOnlyTree) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "metadata lookup must not enumerate files")
}

func TestObjectUnitLocationsDoNotReadBodiesAndKeepPathCompatibility(t *testing.T) {
	member := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}
	aspect := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: member.ObjectID, AspectName: "description"}
	raw, err := EncodeObjectUnitLocations(member.ObjectID, []ObjectUnitLocation{{Address: aspect, Path: "opaque/z.yaml"}, {Address: member, Path: "moved/a.json"}})
	if err != nil {
		t.Fatal(err)
	}
	tree := &metadataOnlyTree{files: map[string][]byte{ObjectLocatorPath(member.ObjectID): raw}}
	locations, err := ReadObjectUnitLocations(tree, member.ObjectID, "fixed")
	want := []ObjectUnitLocation{{Address: member, Path: "moved/a.json"}, {Address: aspect, Path: "opaque/z.yaml"}}
	if err != nil || !reflect.DeepEqual(locations, want) || !reflect.DeepEqual(tree.reads, []string{ObjectLocatorPath(member.ObjectID)}) {
		t.Fatalf("coordinates required bodies or lost identity: got=%#v want=%#v reads=%#v error=%v", locations, want, tree.reads, err)
	}
	paths, err := ReadObjectLocator(tree, member.ObjectID, "fixed")
	if err != nil || !reflect.DeepEqual(paths, []string{"moved/a.json", "opaque/z.yaml"}) {
		t.Fatalf("existing path reader lost compatibility: paths=%#v error=%v", paths, err)
	}
}

func TestPathsOnlyLocatorRequiresExplicitCoordinatesWithoutReadingBodies(t *testing.T) {
	const id objectmodel.ObjectID = "table/orders"
	raw, err := EncodeObjectLocator(id, []string{"opaque/body.json"})
	if err != nil {
		t.Fatal(err)
	}
	tree := &metadataOnlyTree{files: map[string][]byte{ObjectLocatorPath(id): raw}}
	if _, err := ReadObjectUnitLocations(tree, id, "fixed"); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("paths-only locator silently guessed maintenance coordinates: %v", err)
	}
	if !reflect.DeepEqual(tree.reads, []string{ObjectLocatorPath(id)}) {
		t.Fatalf("compatibility lookup read object content: %#v", tree.reads)
	}
	paths, err := ReadObjectLocator(tree, id, "fixed")
	if err != nil || !reflect.DeepEqual(paths, []string{"opaque/body.json"}) {
		t.Fatalf("old exact path access was rejected: paths=%#v error=%v", paths, err)
	}
}

func TestObjectUnitLocationsRetainFullSchemaAdoptionAndKnownEmpty(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "definition"}
	pinned := "schema/table.definition@original-schema-commit"
	noSchema := ""
	for _, ref := range []*string{&pinned, &noSchema} {
		raw, err := EncodeObjectUnitLocations(address.ObjectID, []ObjectUnitLocation{{Address: address, Path: "public.yaml", SchemaRef: ref}})
		if err != nil {
			t.Fatal(err)
		}
		tree := &metadataOnlyTree{files: map[string][]byte{ObjectLocatorPath(address.ObjectID): raw}}
		locations, err := ReadObjectUnitLocations(tree, address.ObjectID, "fixed")
		if err != nil || len(locations) != 1 || locations[0].SchemaRef == nil || *locations[0].SchemaRef != *ref || len(tree.reads) != 1 {
			t.Fatalf("adoption metadata lost its old pin or confused empty with absent: %#v %v", locations, err)
		}
	}
}

func TestObjectUnitLocationsRejectInvalidCoordinates(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}
	for _, tc := range []struct {
		name   string
		change func(*ObjectLocatorEntry)
	}{
		{name: "foreign_object", change: func(entry *ObjectLocatorEntry) { entry.Units[0].Address.ObjectID = "table/other" }},
		{name: "missing_aspect", change: func(entry *ObjectLocatorEntry) { entry.Units[0].Address.AspectName = "" }},
		{name: "outside_path", change: func(entry *ObjectLocatorEntry) {
			entry.Paths[0], entry.Units[0].Path = "../private.json", "../private.json"
		}},
		{name: "path_disagreement", change: func(entry *ObjectLocatorEntry) { entry.Units[0].Path = "private.json" }},
		{name: "invalid_schema_reference", change: func(entry *ObjectLocatorEntry) {
			ref := "not-a-schema"
			entry.Units[0].SchemaRef = &ref
		}},
		{name: "duplicate_address", change: func(entry *ObjectLocatorEntry) {
			entry.Paths = append(entry.Paths, "other.json")
			entry.Units = append(entry.Units, ObjectUnitLocation{Address: address, Path: "other.json"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := ObjectLocatorEntry{ObjectID: address.ObjectID, Paths: []string{"public.json"}, Units: []ObjectUnitLocation{{Address: address, Path: "public.json"}}}
			tc.change(&entry)
			raw, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			tree := &metadataOnlyTree{files: map[string][]byte{ObjectLocatorPath(address.ObjectID): raw}}
			if _, err := ReadObjectUnitLocations(tree, address.ObjectID, "fixed"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("invalid unit coordinates were accepted: %v", err)
			}
			if len(tree.reads) != 1 {
				t.Fatalf("corrupt metadata caused body lookup: %#v", tree.reads)
			}
		})
	}
}

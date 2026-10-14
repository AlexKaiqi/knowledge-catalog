package format

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestSchemaReferrersReadOnlyFixedMetadata(t *testing.T) {
	member := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}
	aspect := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/other", AspectName: "definition"}
	raw, err := EncodeSchemaReferrers("schema/table", []objectmodel.Address{aspect, member})
	if err != nil {
		t.Fatal(err)
	}
	tree := &metadataOnlyTree{files: map[string][]byte{SchemaReferrersPath("schema/table"): raw, LocatorReferrersCompletePath: []byte(LocatorReferrersCompleteBody)}}
	addresses, err := ReadSchemaReferrers(tree, "schema/table", "fixed")
	if err != nil || !reflect.DeepEqual(addresses, []objectmodel.Address{member, aspect}) || !reflect.DeepEqual(tree.reads, []string{SchemaReferrersPath("schema/table")}) {
		t.Fatalf("reverse coordinates read bodies or lost Addresses: got=%#v reads=%v error=%v", addresses, tree.reads, err)
	}
	addresses[0].ObjectID = "mutated"
	again, err := ReadSchemaReferrers(tree, "schema/table", "fixed")
	if err != nil || again[0].ObjectID != member.ObjectID {
		t.Fatalf("caller mutated fixed reverse coordinates: %#v %v", again, err)
	}
	empty, err := ReadSchemaReferrers(tree, "schema/unused", "fixed")
	if err != nil || len(empty) != 0 {
		t.Fatalf("complete metadata did not prove absent reverse entry: %#v %v", empty, err)
	}
}

func TestSchemaReferrersRequireCompletenessOrExplicitLegacyMetadata(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "definition"}
	tree := &metadataOnlyTree{files: map[string][]byte{}}
	if _, err := ReadSchemaReferrers(tree, "schema/table", "fixed"); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing metadata silently became no referrers: %v", err)
	}
	legacy, err := EncodeLocatorManifest(LocatorManifest{Referrers: map[objectmodel.ObjectID][]objectmodel.Address{"schema/table": {address}}})
	if err != nil {
		t.Fatal(err)
	}
	tree.files[LocatorManifestPath] = legacy
	addresses, err := ReadSchemaReferrers(tree, "schema/table", "fixed")
	if err != nil || !reflect.DeepEqual(addresses, []objectmodel.Address{address}) {
		t.Fatalf("explicit old reverse coordinates lost compatibility: %#v %v", addresses, err)
	}
	tree.files[LocatorReferrersCompletePath] = []byte("broken\n")
	if _, err := ReadSchemaReferrers(tree, "schema/table", "fixed"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("corrupt completeness marker fell back to legacy coordinates: %v", err)
	}
}

func TestSchemaReferrersRejectInvalidCoordinates(t *testing.T) {
	valid := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}
	for _, tc := range []struct {
		name   string
		change func(*schemaReferrersEntry)
	}{
		{name: "foreign_schema", change: func(entry *schemaReferrersEntry) { entry.Schema = "schema/other" }},
		{name: "schema_is_not_instance", change: func(entry *schemaReferrersEntry) { entry.Addresses[0].ObjectID = "schema/instance" }},
		{name: "missing_aspect", change: func(entry *schemaReferrersEntry) { entry.Addresses[0].AspectName = "" }},
		{name: "duplicate_address", change: func(entry *schemaReferrersEntry) { entry.Addresses = append(entry.Addresses, valid) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := schemaReferrersEntry{Schema: "schema/table", Addresses: []objectmodel.Address{valid}}
			tc.change(&entry)
			raw, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			tree := &metadataOnlyTree{files: map[string][]byte{SchemaReferrersPath("schema/table"): raw}}
			if _, err := ReadSchemaReferrers(tree, "schema/table", "fixed"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(tree.reads) != 1 {
				t.Fatalf("invalid reverse coordinates triggered fallback or were accepted: reads=%v error=%v", tree.reads, err)
			}
		})
	}
}

package invocation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kc/home"
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestShapedComposedReadReturnsOnlySelectedPartsWithTheirSources(t *testing.T) {
	f := newComposedMemberDeliveryFixture(t)
	flags := composedMemberDeliveryFlags(f)
	flags["reference"] = "urn:kc:object:metric/gmv"
	flags["shape"] = `{ quality { status } lineage }`
	result := f.invoke("object.read", flags, VerbRead)
	if result.Status != 0 {
		t.Fatalf("shaped composed READ failed: %#v", result)
	}
	var got objectmodel.ShapedValue
	if err := json.Unmarshal([]byte(result.Stdout), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"quality": map[string]any{"status": "current"}, "lineage": nil}
	if !reflect.DeepEqual(got.Data, want) {
		t.Fatalf("shape returned unselected or wrong parts: %#v", got.Data)
	}
	if len(got.Errors) != 1 || !reflect.DeepEqual(got.Errors[0].Path, []string{"lineage"}) || got.Errors[0].Code != kernel.ErrObjectRefUnresolved {
		t.Fatalf("missing aspect must be a located error: %#v", got.Errors)
	}
	quality := f.repos[1].ID()
	if len(got.Sources) != 1 || got.Sources[0].Repository != quality || got.Sources[0].Commit != f.pin.Repositories[quality] {
		t.Fatalf("shape must report only the source of the selected unit: %#v", got.Sources)
	}
}

func TestShapedReadEnforcesTheAdoptedClosedSchema(t *testing.T) {
	s := testkit.NewSetup(t, "kr://invocation/shape-schema")
	schema := func(id string, closed bool) objectmodel.Operation {
		body := map[string]any{"metaSchema": string(objectmodel.MetaSchemaV1), "entity": "Policy", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{"name": map[string]any{"type": "string", "required": true}}}
		if closed {
			body["additionalProperties"] = false
		}
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, Value: body}
	}
	unit := func(object, schemaRef string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectmodel.ObjectID(object), AspectName: "structure"}, SchemaRef: schemaRef, Value: map[string]any{"name": object}}
	}
	if _, err := s.Writer.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{schema("schema/policy/closed", true), schema("schema/policy/open", false),
			unit("policy/closed", "schema/policy/closed"), unit("policy/open", "schema/policy/open")},
	}); err != nil {
		t.Fatal(err)
	}
	ws := &home.Home{Store: s.Store, Writer: s.Writer, Reader: s.Reader}
	read := func(object, shape string) (any, error) {
		return VerbRead(&Invocation{WS: ws, Context: context.Background(), Flags: map[string]invoke.FlagValue{
			"reference": "urn:kc:object:" + object, "repo": string(s.RepositoryID), "shape": shape}})
	}
	if closedRaw, err := read("policy/closed", `{ structure { name owner } }`); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("closed Schema must refuse an undeclared field: %#v %v", closedRaw, err)
	}
	if _, err := read("policy/open", `{ structure { name { first } } }`); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("a sub-selection under a string field must refuse even when the Schema is open: %v", err)
	}
	raw, err := read("policy/open", `{ structure { name owner } }`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(objectmodel.ShapedValue)
	want := map[string]any{"structure": map[string]any{"name": "policy/open", "owner": nil}}
	if !reflect.DeepEqual(got.Data, want) || len(got.Errors) != 1 || !reflect.DeepEqual(got.Errors[0].Path, []string{"structure", "owner"}) {
		t.Fatalf("open Schema must report the absent field at its path: %#v", got)
	}
}

func TestShapedReadDigestDrivesConditionalUpdate(t *testing.T) {
	s := testkit.NewSetup(t, "kr://invocation/shape-cas")
	unit := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/one", AspectName: "structure"}
	seed, err := s.Writer.Commit("seed", objectmodel.ChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: unit, Value: map[string]any{"name": "old", "priority": 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ws := &home.Home{Store: s.Store, Writer: s.Writer, Reader: s.Reader}
	shaped := func() objectmodel.ShapedValue {
		raw, err := VerbRead(&Invocation{WS: ws, Context: context.Background(), Flags: map[string]invoke.FlagValue{
			"reference": "urn:kc:object:policy/one", "repo": string(s.RepositoryID), "shape": "{ structure { name } }"}})
		if err != nil {
			t.Fatal(err)
		}
		return raw.(objectmodel.ShapedValue)
	}
	put := func(command string, digest kernel.Digest, value string) error {
		_, err := VerbPut(&Invocation{WS: ws, Context: context.Background(), Flags: map[string]invoke.FlagValue{
			"reference": "urn:kc:object:policy/one#/structure", "repo": string(s.RepositoryID), "command-id": command,
			"if-digest": string(digest), "value": value}})
		return err
	}
	before := shaped()
	whole, err := s.Reader.ReadAddress(s.RepositoryID, unit, seed.Result.NewCommit)
	if err != nil || len(before.Declarations) != 1 || before.Declarations[0].Digest != whole.Digest {
		t.Fatalf("shaped read must carry the whole unit digest: %#v %v", before.Declarations, err)
	}
	if err := put("bump-priority", before.Declarations[0].Digest, `{"name":"old","priority":2}`); err != nil {
		t.Fatalf("unit digest from a shaped read must drive a conditional update: %v", err)
	}
	if !reflect.DeepEqual(shaped().Data, before.Data) {
		t.Fatal("unselected field change must not alter the shaped data")
	}
	if err := put("stale", before.Declarations[0].Digest, `{"name":"stale"}`); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("digest must go stale when an unselected field of the unit changed: %v", err)
	}
}

func TestShapedReadRefusesLocatorAndInvalidShapesBeforeReading(t *testing.T) {
	located := map[string]invoke.FlagValue{"reference": "urn:kc:object:table/orders#/columns", "shape": "{ definition }"}
	if err := NormalizeReferenceFlags(located); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("locator plus shape must refuse: %v", err)
	}
	f := newComposedMemberDeliveryFixture(t)
	for _, shape := range []string{`{ definition @skip(if: true) }`, `{ a: definition }`, `query Q { definition }`, `{`} {
		flags := composedMemberDeliveryFlags(f)
		flags["reference"] = "urn:kc:object:metric/gmv"
		flags["shape"] = shape
		if code := namedConsumptionError(t, f.invoke("object.read", flags, VerbRead)); code != kernel.ErrUsageInvalid {
			t.Fatalf("%s: got %s", shape, code)
		}
	}
}

package reader_test

import (
	"fmt"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

func fixedMemberSourceFixture(t *testing.T, bound bool) (snapshot.Store, *writer.Writer, kernel.CommitID, objectmodel.Address) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://reader/member-source")
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "widget/selected", AspectName: "readings", MemberKey: "good"}
	receipt, err := w.Commit("publish-member-source", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.readings"}, PathHint: "private/schema.yaml", Value: memberSchema("readings")},
			bindingfixture.PutDescriptor("resource/widget-source", bindingfixture.Descriptor("https://source.example")),
			memberUnit(address, "schema/widget.readings", bound),
		}})
	if err != nil {
		t.Fatal(err)
	}
	return raw, w, receipt.Result.NewCommit, address
}

func memberSchema(description string) map[string]any {
	return map[string]any{"entity": "Widget", "aspect": "readings", "pattern": "keyed_collection", "description": description,
		"fields": map[string]any{"value": map[string]any{"type": "number"}}}
}

// memberUnit declares the Member's source on the unit itself: a binding unit
// names a ResourceDescriptor and carries no value.
func memberUnit(address objectmodel.Address, schemaRef string, bound bool) objectmodel.Operation {
	if bound {
		op := bindingfixture.Bind(address, schemaRef, "resource/widget-source")
		op.PathHint = "public/member.yaml"
		return op
	}
	return objectmodel.Operation{Op: objectmodel.OpPut, Address: address, SchemaRef: schemaRef, PathHint: "public/member.yaml", Value: map[string]any{"value": float64(31)}}
}

// The source category is the unit's own fixed declaration: changing the
// current Schema neither flips it nor requires reading Schema bodies.
func TestMemberSourceClassificationFollowsUnitDeclaration(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint("bound=", bound), func(t *testing.T) {
			raw, w, original, address := fixedMemberSourceFixture(t, bound)
			newer, err := w.Commit("change-current-schema", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
				BaseCommit: original, ExpectedTargetCommit: original, Operations: []objectmodel.Operation{
					{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/widget.readings"}, Value: memberSchema("changed readings")},
					memberUnit(address, "schema/widget.readings@"+string(original), bound),
				}})
			if err != nil {
				t.Fatal(err)
			}
			counting := &locatorCountingRepository{Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string]int{}}
			repo, err := reader.NewReader(nil).Wrap(counting, kernel.ErrCapabilityUnsatisfied)
			if err != nil {
				t.Fatal(err)
			}
			at := newer.Result.NewCommit
			value, err := repo.ReadAddress(address, at)
			if err != nil {
				t.Fatal(err)
			}
			assertMemberCategory(t, value.Declarations, bound)
			whole, err := repo.Read(address.ObjectID, at)
			if err != nil {
				t.Fatal(err)
			}
			assertMemberCategory(t, whole.Declarations, bound)
			batch, err := repo.(objectmodel.BatchReadStore).ReadMany([]objectmodel.ObjectID{address.ObjectID}, at)
			if err != nil {
				t.Fatal(err)
			}
			assertMemberCategory(t, batch[address.ObjectID].Declarations, bound)
			ref := objectmodel.Reference{Repository: raw.ID(), Object: address.ObjectID, Locator: "/readings/good/value"}
			units, err := repo.(objectmodel.ReferenceUnitsReader).ReadReferenceUnits(ref, at)
			if err != nil {
				t.Fatal(err)
			}
			assertMemberCategory(t, objectmodel.DeclareUnits(units), bound)
			if counting.reads["private/schema.yaml"] != 0 || counting.listCalls != 0 {
				t.Fatalf("source classification read Schema bodies or scanned the authority: %#v lists=%d", counting.reads, counting.listCalls)
			}
		})
	}
}

func assertMemberCategory(t *testing.T, declarations []objectmodel.UnitDeclaration, bound bool) {
	t.Helper()
	if len(declarations) != 1 {
		t.Fatalf("wrong selected declarations: %#v", declarations)
	}
	source := declarations[0].ValueSource
	if (source != nil && source.Kind == objectmodel.ValueSourceBinding) != bound {
		t.Fatalf("Member source must follow the unit valueSource: %#v", declarations)
	}
}

// A Member's category is its own declaration. A snapshot Member adopting the
// same Schema as a binding Member stays a snapshot value.
func TestMemberSourceIsNotInferredFromSharedSchema(t *testing.T) {
	raw, w, commit, address := fixedMemberSourceFixture(t, true)
	plain := address
	plain.MemberKey = "plain"
	next, err := w.Commit("publish-snapshot-member", objectmodel.ChangeSet{TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: commit, ExpectedTargetCommit: commit, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: plain, SchemaRef: "schema/widget.readings", PathHint: "public/plain.yaml", Value: map[string]any{"value": float64(7)}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.ReadAddress(plain, next.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberCategory(t, value.Declarations, false)
	bound, err := repo.ReadAddress(address, next.Result.NewCommit)
	if err != nil {
		t.Fatal(err)
	}
	assertMemberCategory(t, bound.Declarations, true)
}

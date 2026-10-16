package invocation

import (
	"testing"

	objectmodel "kc/object"
)

func TestComposedAccessExtractionUsesSourceVector(t *testing.T) {
	const reference = "urn:kc:object:table/orders"
	definition := objectmodel.ObjectValue{
		ObjectRef: objectmodel.ObjectRef{Object: "table/orders"}, Repository: "kr://audit/a", Commit: "a1",
		Value: map[string]any{"definition": map[string]any{"repository": "kr://fake/payload", "commit": "fake", "objectId": "fake/value"}},
	}
	columns := objectmodel.ObjectValue{
		ObjectRef: objectmodel.ObjectRef{Object: "table/orders"}, Repository: "kr://audit/b", Commit: "b1",
		Value: map[string]any{"columns": map[string]any{"id": map[string]any{"type": "string"}, "amount": map[string]any{"type": "number"}}},
		Units: []objectmodel.Address{
			{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"},
			{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "amount"},
		},
	}
	composed, err := objectmodel.ComposeHoldingValues(reference, []objectmodel.ObjectValue{definition, columns})
	if err != nil {
		t.Fatal(err)
	}
	// Repeated envelopes deduplicate, while a collection remains one adopted Aspect scope.
	targets := ObjectAccesses([]objectmodel.ObjectValue{composed, composed})
	if len(targets) != 2 {
		t.Fatalf("composed source evidence is missing or duplicated: %#v", targets)
	}
	for i, want := range []struct{ repo, commit, aspect string }{{"kr://audit/a", "a1", "definition"}, {"kr://audit/b", "b1", "columns"}} {
		got := targets[i]
		if string(got.ObjectRef.Repository) != want.repo || string(got.ObjectRef.Commit) != want.commit || got.ObjectRef.Object != "table/orders" ||
			got.Reference != reference || got.Address == nil || *got.Address != (objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: want.aspect}) {
			t.Fatalf("source %d lost its fixed coordinates or scope: %#v", i, got)
		}
	}
}

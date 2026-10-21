package reader_test

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	objectservice "kc/object/service"
)

// U21: a shape never reads an unselected unit body, and trims fields from the
// unit it did read before the result is assembled.
func TestShapeReadLoadsOnlySelectedUnitBodies(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	guard.blocked["private/description.yaml"] = true
	guard.blocked["private/status.yaml"] = true
	executor := objectservice.ShapeReadExecutor{
		Read: func(_ context.Context, ref objectmodel.Reference) (objectmodel.ObservedValue, error) {
			ref.Repository = guard.ID()
			value, err := rd.ReadReference(ref, at)
			if err != nil {
				return objectmodel.ObservedValue{}, err
			}
			return objectmodel.ObservedValue{FederatedValue: objectmodel.FederatedValue{
				Repository: value.Repository, Commit: value.Commit, Value: value.Value, Declarations: value.Declarations,
			}}, nil
		},
		Schema: func(context.Context, kernel.RepositoryID, kernel.CommitID, string) (*objectmodel.SchemaDefinition, error) {
			return nil, nil
		},
	}
	got, err := executor.Execute(context.Background(), objectservice.ShapeReadRequest{
		Reference: objectmodel.Reference{Object: "table/orders"},
		Shape:     `{ columns(keys: ["order_id", "ghost"]) { type constraints { nullable } } }`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"columns": map[string]any{
		"order_id": map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}},
		"ghost":    nil,
	}}
	if !reflect.DeepEqual(got.Data, want) {
		t.Fatalf("data = %#v", got.Data)
	}
	if len(got.Errors) != 1 || got.Errors[0].Code != kernel.ErrObjectRefUnresolved || !reflect.DeepEqual(got.Errors[0].Path, []string{"columns", "ghost"}) {
		t.Fatalf("errors = %#v", got.Errors)
	}
	if guard.reads["public/id.yaml"] != 1 || guard.reads["private/description.yaml"] != 0 || guard.reads["private/status.yaml"] != 0 {
		t.Fatalf("shape read unselected unit bodies: %#v", guard.reads)
	}
	complete := kernel.CanonicalDigest(map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}, "optional": nil})
	if len(got.Declarations) != 1 || got.Declarations[0].Digest != complete {
		t.Fatalf("trimmed member must keep its complete maintenance digest: %#v", got.Declarations)
	}
}

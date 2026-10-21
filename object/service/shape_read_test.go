package service

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

type shapeFixture struct {
	reads   []string
	values  map[string]objectmodel.ObservedValue
	fails   map[string]error
	schemas map[string]*objectmodel.SchemaDefinition
}

func (f *shapeFixture) executor() ShapeReadExecutor {
	return ShapeReadExecutor{
		Read: func(_ context.Context, ref objectmodel.Reference) (objectmodel.ObservedValue, error) {
			f.reads = append(f.reads, ref.Locator)
			if err := f.fails[ref.Locator]; err != nil {
				return objectmodel.ObservedValue{}, err
			}
			value, ok := f.values[ref.Locator]
			if !ok {
				return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "reference does not select published maintenance content")
			}
			return value, nil
		},
		Schema: func(_ context.Context, _ kernel.RepositoryID, _ kernel.CommitID, ref string) (*objectmodel.SchemaDefinition, error) {
			return f.schemas[ref], nil
		},
	}
}

func unit(kind objectmodel.AddressKind, aspect, key, schemaRef string, digest kernel.Digest) objectmodel.UnitDeclaration {
	return objectmodel.UnitDeclaration{Address: objectmodel.Address{Kind: kind, ObjectID: "trade/order-service", AspectName: aspect, MemberKey: key}, Digest: digest, SchemaRef: schemaRef}
}

func observed(value any, declarations ...objectmodel.UnitDeclaration) objectmodel.ObservedValue {
	return objectmodel.ObservedValue{FederatedValue: objectmodel.FederatedValue{Repository: "kr://acme/services", Commit: "c1", Value: value, Declarations: declarations}}
}

func orderService() *shapeFixture {
	return &shapeFixture{
		values: map[string]objectmodel.ObservedValue{
			"/definition": observed(map[string]any{"name": "order", "purpose": "orders"}, unit(objectmodel.KindAspect, "definition", "", "", "d-def")),
			"/interfaces/create-order": observed(map[string]any{"path": "/orders", "owner": nil, "body": "large"},
				unit(objectmodel.KindMember, "interfaces", "create-order", "schema/service.interface", "d-create")),
			"/interfaces": observed(map[string]any{
				"create-order": map[string]any{"path": "/orders", "body": "large"},
				"cancel-order": map[string]any{"path": "/orders/cancel", "body": "large"},
			}, unit(objectmodel.KindMember, "interfaces", "create-order", "", "d-create"), unit(objectmodel.KindMember, "interfaces", "cancel-order", "", "d-cancel")),
		},
		fails:   map[string]error{},
		schemas: map[string]*objectmodel.SchemaDefinition{},
	}
}

func execute(t *testing.T, f *shapeFixture, shape string) (objectmodel.ShapedValue, error) {
	t.Helper()
	return f.executor().Execute(context.Background(), ShapeReadRequest{Reference: objectmodel.Reference{Object: "trade/order-service"}, Shape: shape})
}

func TestShapeReadReadsOnlySelectedUnitsAndTrimsBeforeAssembly(t *testing.T) {
	f := orderService()
	got, err := execute(t, f, `{ definition interfaces(keys: ["create-order"]) { path owner } }`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/definition", "/interfaces/create-order"}; !reflect.DeepEqual(f.reads, want) {
		t.Fatalf("reads = %v, want only the selected units %v", f.reads, want)
	}
	want := map[string]any{
		"definition": map[string]any{"name": "order", "purpose": "orders"},
		"interfaces": map[string]any{"create-order": map[string]any{"path": "/orders", "owner": nil}},
	}
	if !reflect.DeepEqual(got.Data, want) {
		t.Fatalf("data = %#v", got.Data)
	}
	if len(got.Errors) != 0 {
		t.Fatalf("legitimate null has no error, got %#v", got.Errors)
	}
	digests := []string{}
	for _, declaration := range got.Declarations {
		digests = append(digests, string(declaration.Digest))
	}
	if !reflect.DeepEqual(digests, []string{"d-def", "d-create"}) {
		t.Fatalf("each read unit keeps its complete maintenance digest, got %v", digests)
	}
	if len(got.Sources) != 1 || !reflect.DeepEqual(got.Sources[0].Aspects, []string{"definition", "interfaces"}) {
		t.Fatalf("sources = %#v", got.Sources)
	}
}

func TestShapeReadProjectsEveryMemberWhenKeysAreOmitted(t *testing.T) {
	f := orderService()
	got, err := execute(t, f, `{ interfaces { path } }`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"interfaces": map[string]any{
		"create-order": map[string]any{"path": "/orders"},
		"cancel-order": map[string]any{"path": "/orders/cancel"},
	}}
	if !reflect.DeepEqual(got.Data, want) {
		t.Fatalf("data = %#v", got.Data)
	}
}

func TestShapeReadReportsLocatedFailuresWithoutHidingOtherSelections(t *testing.T) {
	f := orderService()
	f.fails["/runbook"] = kernel.Fail(kernel.ErrForbidden, "held only by Repositories the caller may not read")
	f.fails["/monitoring"] = kernel.Fail(kernel.ErrObjectIDConflict, "aspect monitoring comes from two Repositories")
	got, err := execute(t, f, `{ definition runbook monitoring interfaces(keys: ["ghost"]) }`)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]kernel.ErrorCode{}
	for _, e := range got.Errors {
		codes[pathKey(e.Path)] = e.Code
	}
	want := map[string]kernel.ErrorCode{
		"runbook":          kernel.ErrForbidden,
		"monitoring":       kernel.ErrObjectIDConflict,
		"interfaces/ghost": kernel.ErrObjectRefUnresolved,
	}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("errors = %#v", got.Errors)
	}
	if got.Data["runbook"] != nil || got.Data["monitoring"] != nil || got.Data["definition"] == nil {
		t.Fatalf("data = %#v", got.Data)
	}
}

func TestShapeReadRejectsWholeRequest(t *testing.T) {
	t.Run("closed schema", func(t *testing.T) {
		f := orderService()
		f.schemas["schema/service.interface"] = &objectmodel.SchemaDefinition{ObjectID: "schema/service.interface", Fields: []objectmodel.SchemaFieldDefinition{{Path: "path"}, {Path: "owner"}, {Path: "body"}}}
		if _, err := execute(t, f, `{ interfaces(keys: ["create-order"]) { path ghost } }`); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("unknown field on closed schema err = %v", err)
		}
	})
	t.Run("keys on a record aspect", func(t *testing.T) {
		f := orderService()
		f.values["/definition/name"] = observed("order", unit(objectmodel.KindAspect, "definition", "", "", "d-def"))
		if _, err := execute(t, f, `{ definition(keys: ["name"]) }`); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("keys on record err = %v", err)
		}
	})
	t.Run("reference locator with shape", func(t *testing.T) {
		f := orderService()
		_, err := f.executor().Execute(context.Background(), ShapeReadRequest{Reference: objectmodel.Reference{Object: "trade/order-service", Locator: "/definition"}, Shape: `{ definition }`})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(f.reads) != 0 {
			t.Fatalf("locator plus shape err = %v reads = %v", err, f.reads)
		}
	})
	t.Run("invalid shape reads nothing", func(t *testing.T) {
		f := orderService()
		if _, err := execute(t, f, `{ d: definition }`); kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(f.reads) != 0 {
			t.Fatalf("alias err = %v reads = %v", err, f.reads)
		}
	})
	t.Run("unlocated failure", func(t *testing.T) {
		f := orderService()
		f.fails["/definition"] = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository has no metadata-only reference planner")
		if _, err := execute(t, f, `{ definition }`); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("capability failure must not become a null, err = %v", err)
		}
	})
}

func TestShapeReadReportsExternalFailureAtItsPath(t *testing.T) {
	f := orderService()
	value := observed(map[string]any{}, unit(objectmodel.KindAspect, "health", "", "", "d-health"))
	value.Failures = []objectmodel.UnitReadFailure{{Address: value.Declarations[0].Address, Code: kernel.ErrorCode("SOURCE_UNAVAILABLE"), Message: "source offline"}}
	f.values["/health"] = value
	got, err := execute(t, f, `{ health { status } }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Errors) != 1 || pathKey(got.Errors[0].Path) != "health" || got.Data["health"] != nil || len(got.Failures) != 1 {
		t.Fatalf("got = %#v", got)
	}
}

func pathKey(path []string) string {
	out := ""
	for i, token := range path {
		if i > 0 {
			out += "/"
		}
		out += token
	}
	return out
}

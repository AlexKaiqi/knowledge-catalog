package projection

import (
	ports "kc/discovery/projection"
	"reflect"
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/bindingfixture"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func TestCompiledDocumentDoesNotCarryWorkspaceScope(t *testing.T) {
	typeOfDoc := reflect.TypeOf(ports.CompiledDoc{})
	for i := 0; i < typeOfDoc.NumField(); i++ {
		field := typeOfDoc.Field(i)
		name := strings.ToLower(field.Name + " " + strings.Split(field.Tag.Get("json"), ",")[0])
		for _, forbidden := range []string{"workspace", "pinid", "pin_id"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("CompiledDoc must remain reusable across Workspaces; found request scope field %s", field.Name)
			}
		}
	}
}

func TestProjectionCompilerRequiresObservationForBindingEligibility(t *testing.T) {
	field := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/live", Aspect: "status", Path: "owner"},
		Type:     "string", Access: []reader.AccessHint{reader.HintFilter},
	}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "status"}
	source := bindingfixture.Source("resource/live")
	declarationDigest := objectmodel.BindingDeclarationDigest(objectmodel.DeclarationDigest("schema/live", source), "descriptor-digest")
	value := objectmodel.ObjectValue{
		Commit: "c1", Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "table/orders"},
		Value: map[string]any{"status": nil},
		Declarations: []objectmodel.UnitDeclaration{{
			Address: address, SchemaRef: "schema/live", DeclarationDigest: declarationDigest,
			ValueSource: source,
		}},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.EligibleFields) != 0 {
		t.Fatalf("unobserved Binding must not prove MISSING: %#v", doc.EligibleFields)
	}

	doc, err = compileProjectionDocumentObserved(nil, value, []objectmodel.UnitObservation{{
		Address: address, DeclarationCommit: "c1", DeclarationDigest: declarationDigest,
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}}, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 0 || len(doc.EligibleFields) != 1 || doc.EligibleFields[0] != field.FieldRef.Key() {
		t.Fatalf("observed null must prove MISSING without a cell: %#v", doc)
	}

	_, err = compileProjectionDocumentObserved(nil, value, []objectmodel.UnitObservation{{
		Address: address, DeclarationCommit: "c1", DeclarationDigest: kernel.Digest("wrong"),
	}}, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("mismatched observation must be rejected, got %v", err)
	}
}

func TestProjectionCompilerTreatsMembersAsMaintenanceUnits(t *testing.T) {
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{{
		FieldRef: retrieval.FieldRef{Schema: "schema/tag", Aspect: "tags", Path: "label"},
		Type:     "string", Access: []reader.AccessHint{reader.HintFilter, reader.HintText},
	}}}
	value := objectmodel.ObjectValue{
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "table/orders"},
		Value: map[string]any{"tags": map[string]any{
			"owner": map[string]any{"label": "finance"},
			"tier":  map[string]any{"label": "gold"},
		}},
		Declarations: []objectmodel.UnitDeclaration{
			{Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "tags", MemberKey: "owner"}, SchemaRef: "schema/tag"},
			{Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "tags", MemberKey: "tier"}, SchemaRef: "schema/tag"},
		},
	}
	doc, err := compileProjectionDocument(nil, value, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 2 || doc.Cells[0].Value != "finance" || doc.Cells[1].Value != "gold" {
		t.Fatalf("member values must be flattened under one object document: %#v", doc.Cells)
	}
	if len(doc.EligibleFields) != 1 || doc.EligibleFields[0] != spec.Fields[0].FieldRef.Key() {
		t.Fatalf("eligible fields: %#v", doc.EligibleFields)
	}
}

func TestProjectionCompilerPreservesMissingApplicability(t *testing.T) {
	field := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/table", Aspect: "structure", Path: "owner"},
		Type:     "string", Access: []reader.AccessHint{reader.HintFilter},
	}
	value := objectmodel.ObjectValue{
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "table/orders"},
		Value:   map[string]any{"structure": map[string]any{"name": "orders"}},
		Declarations: []objectmodel.UnitDeclaration{{
			Address:   objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "structure"},
			SchemaRef: "schema/table",
		}},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 0 || len(doc.EligibleFields) != 1 || doc.EligibleFields[0] != field.FieldRef.Key() {
		t.Fatalf("missing value must keep applicability without inventing a cell: %#v", doc)
	}
}

func TestProjectionCompilerEmitsRelationCore(t *testing.T) {
	value := objectmodel.ObjectValue{
		Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation/owned"},
		Value: map[string]any{
			"relationId": "relation/owned", "relationType": "owned-by", "direction": "DIRECTED",
			"endpoints": []any{
				map[string]any{"role": "subject", "object": "table/orders"},
				map[string]any{"role": "owner", "object": "team/finance"},
			},
		},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != objectmodel.KindRelation || doc.Relation == nil || doc.Relation.Type != "owned-by" || len(doc.Relation.Endpoints) != 2 {
		t.Fatalf("relation core missing: %#v", doc)
	}
}

func TestProjectionCompilerRejectsInvalidTypedValue(t *testing.T) {
	value := objectmodel.ObjectValue{
		Address:      objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/P-1"},
		Value:        map[string]any{"score": "not-a-number"},
		Declarations: []objectmodel.UnitDeclaration{{Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/P-1"}, SchemaRef: "schema/policy"}},
	}
	_, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{{
		FieldRef: retrieval.FieldRef{Schema: "schema/policy", Path: "score"}, Type: "number", Access: []reader.AccessHint{reader.HintFilter},
	}}})
	if err == nil {
		t.Fatal("invalid typed value must fail projection instead of silently reducing coverage")
	}
}

func TestProjectionCompilerIndexesObjectReferenceAsKeyword(t *testing.T) {
	field := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/table", Aspect: "properties", Path: "platformRef"},
		Type:     "object_ref", Access: []reader.AccessHint{reader.HintFilter},
	}
	value := objectmodel.ObjectValue{
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "table/orders"},
		Value:   map[string]any{"properties": map[string]any{"platformRef": "platform/warehouse"}},
		Declarations: []objectmodel.UnitDeclaration{{
			Address:   objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "properties"},
			SchemaRef: "schema/table",
		}},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 1 || doc.Cells[0].StringValue == nil || *doc.Cells[0].StringValue != "platform/warehouse" {
		t.Fatalf("object reference cell: %#v", doc.Cells)
	}
}

func TestProjectionCompilerIndexesReadmeBodyAsText(t *testing.T) {
	field := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: objectmodel.ObjectID("schema/core/readme/v1"), Aspect: "readme", Path: "body"},
		Type:     "string", Access: []reader.AccessHint{reader.HintText},
	}
	body := "# Payments warehouse\n\nPublished metrics."
	value := objectmodel.ObjectValue{
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "fixture/payments"},
		Value:   map[string]any{"readme": map[string]any{"body": body}},
		Declarations: []objectmodel.UnitDeclaration{{
			Address:   objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: "readme"},
			SchemaRef: string(objectmodel.ObjectID("schema/core/readme/v1")),
		}},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Text != body {
		t.Fatalf("README SEARCH must index the object body, not a file path: %#v", doc)
	}
	if len(doc.Cells) != 1 || doc.Cells[0].TextValue != body || doc.Cells[0].StringValue != nil {
		t.Fatalf("body is text access, not a keyword term: %#v", doc.Cells)
	}
}

// Stream Bindings never produce an ordinary observation (READ rejects them
// before any lookup), so the compiler sees them as unobserved Binding units:
// content at that position in the fixed value never becomes a document cell.
func TestProjectionCompilerSkipsUnobservedBindingContent(t *testing.T) {
	field := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/live", Aspect: "events", Path: "last"},
		Type:     "string", Access: []reader.AccessHint{reader.HintFilter},
	}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/orders", AspectName: "events"}
	source := bindingfixture.Source("resource/events")
	value := objectmodel.ObjectValue{
		Commit: "c1", Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "job/orders"},
		Value: map[string]any{"events": map[string]any{"last": "should-not-index"}},
		Declarations: []objectmodel.UnitDeclaration{{
			Address: address, SchemaRef: "schema/live", DeclarationDigest: objectmodel.DeclarationDigest("schema/live", source), ValueSource: source,
		}},
	}
	doc, err := compileProjectionDocumentObserved(nil, value, nil, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 0 || len(doc.EligibleFields) != 0 {
		t.Fatalf("unobserved Binding content must not become a snapshot object document: %#v", doc)
	}
}

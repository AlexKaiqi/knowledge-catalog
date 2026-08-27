package index

import (
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

func TestCompiledDocumentDoesNotCarryWorkspaceScope(t *testing.T) {
	typeOfDoc := reflect.TypeOf(CompiledDoc{})
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
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:orders", AspectName: "status"}
	declarationDigest := knowledge.DeclarationDigest("schema/live", &knowledge.ValueSource{
		Kind:    knowledge.ValueSourceBinding,
		Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, Runtime: "fixture", Protocol: "resource-access/v1", Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "status"}}},
	})
	value := knowledge.KnowledgeValue{
		Commit: "c1", Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Table:orders"},
		Value: map[string]any{"status": nil},
		Declarations: []knowledge.UnitDeclaration{{
			Address: address, SchemaRef: "schema/live", DeclarationDigest: declarationDigest,
			ValueSource: &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState}},
		}},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.EligibleFields) != 0 {
		t.Fatalf("unobserved Binding must not prove MISSING: %#v", doc.EligibleFields)
	}

	doc, err = compileProjectionDocumentObserved(nil, value, []knowledge.UnitObservation{{
		Address: address, DeclarationCommit: "c1", DeclarationDigest: declarationDigest,
		Basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationRepeatable, SourceRevision: "r1", ObservedAt: "2026-08-27T00:00:00Z"},
	}}, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 0 || len(doc.EligibleFields) != 1 || doc.EligibleFields[0] != field.FieldRef.Key() {
		t.Fatalf("observed null must prove MISSING without a cell: %#v", doc)
	}

	_, err = compileProjectionDocumentObserved(nil, value, []knowledge.UnitObservation{{
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
	value := knowledge.KnowledgeValue{
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Table:orders"},
		Value: map[string]any{"tags": map[string]any{
			"owner": map[string]any{"label": "finance"},
			"tier":  map[string]any{"label": "gold"},
		}},
		Declarations: []knowledge.UnitDeclaration{
			{Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "Table:orders", AspectName: "tags", MemberKey: "owner"}, SchemaRef: "schema/tag"},
			{Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "Table:orders", AspectName: "tags", MemberKey: "tier"}, SchemaRef: "schema/tag"},
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
	value := knowledge.KnowledgeValue{
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Table:orders"},
		Value:   map[string]any{"structure": map[string]any{"name": "orders"}},
		Declarations: []knowledge.UnitDeclaration{{
			Address:   knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:orders", AspectName: "structure"},
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
	value := knowledge.KnowledgeValue{
		Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation:owned"},
		Value: map[string]any{
			"relationId": "relation:owned", "relationType": "owned-by", "direction": "DIRECTED",
			"endpoints": []any{
				map[string]any{"role": "subject", "objectRef": "Table:orders"},
				map[string]any{"role": "owner", "objectRef": "Team:finance"},
			},
		},
	}
	doc, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != knowledge.KindRelation || doc.Relation == nil || doc.Relation.Type != "owned-by" || len(doc.Relation.Endpoints) != 2 {
		t.Fatalf("relation core missing: %#v", doc)
	}
}

func TestProjectionCompilerRejectsInvalidTypedValue(t *testing.T) {
	value := knowledge.KnowledgeValue{
		Address:      knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Policy:P-1"},
		Value:        map[string]any{"score": "not-a-number"},
		Declarations: []knowledge.UnitDeclaration{{Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Policy:P-1"}, SchemaRef: "schema/policy"}},
	}
	_, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{{
		FieldRef: retrieval.FieldRef{Schema: "schema/policy", Path: "score"}, Type: "number", Access: []reader.AccessHint{reader.HintFilter},
	}}})
	if err == nil {
		t.Fatal("invalid typed value must fail projection instead of silently reducing coverage")
	}
}

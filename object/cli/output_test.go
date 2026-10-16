package cli

import (
	"reflect"
	"testing"
)

func TestSelectiveReadKeepsConditionalMaintenanceEvidence(t *testing.T) {
	raw := map[string]any{"reference": testReference, "locator": "/columns/order_id/constraints/nullable", "repository": "kr://scene/object", "commit": "c1", "address": map[string]any{"objectId": "table/orders", "aspectName": "columns", "memberKey": "order_id", "kind": "Member"}, "digest": "full-member-digest", "value": false, "declarations": []any{map[string]any{"schemaRef": "pinned-schema", "provenance": map[string]any{"sourceRefs": []any{"source"}}}}}
	got := Shape("read", raw).(map[string]any)
	for _, key := range []string{"reference", "locator", "repository", "commit", "address", "digest", "value", "declarations"} {
		if !reflect.DeepEqual(got[key], raw[key]) {
			t.Fatalf("lost %s: %#v", key, got)
		}
	}
	raw["value"] = nil
	got = Shape("read", raw).(map[string]any)
	if value, present := got["value"]; !present || value != nil {
		t.Fatal("null became missing")
	}
}

func TestSchemaDescribeKeepsCompleteDefinition(t *testing.T) {
	definition := map[string]any{"entity": "Order", "additionalProperties": false, "fields": map[string]any{"name": map[string]any{"type": "string", "required": true, "minLength": 1}}}
	raw := map[string]any{"repository": "r", "commit": "c1", "schemas": []any{map[string]any{"objectId": "schema/order/v1", "definition": definition, "fields": []any{map[string]any{"path": "name", "type": "string"}}}}}
	got := Shape("schema describe", raw).(map[string]any)["schemas"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(got["definition"], definition) {
		t.Fatalf("lost structural constraints: %#v", got)
	}
}

func TestDatasetSchemaDescribeKeepsRevisionAndSearchReadiness(t *testing.T) {
	search := map[string]any{"state": "unavailable", "reason": "projection is building", "declarations": []any{map[string]any{"member": "index/item#index/name", "status": "ready"}}}
	raw := []any{map[string]any{"dataset": "agent", "revision": float64(3), "search": search, "repository": "r", "commit": "c1", "schemas": []any{}}}
	got := Shape("schema describe", raw).([]any)[0].(map[string]any)
	if got["dataset"] != "agent" || got["revision"] != float64(3) || !reflect.DeepEqual(got["search"], search) {
		t.Fatalf("Dataset describe lost the answering revision or search readiness: %#v", got)
	}
}

func TestResolveKeepsItsOwnProductShape(t *testing.T) {
	raw := map[string]any{"repository": "r", "commit": "c1", "status": "RESOLVED", "digest": "d1", "address": map[string]any{"objectId": "note/hello", "aspectName": "readme", "memberKey": "user:bob"}, "reference": testReference, "locator": "/readme/user:bob", "declarations": []any{"internal"}, "provenance": map[string]any{"actor": "internal"}, "pathHint": "notes/hello.yaml"}
	want := map[string]any{"repository": "r", "commit": "c1", "objectId": "note/hello", "aspectName": "readme", "memberKey": "user:bob", "status": "RESOLVED", "digest": "d1"}
	if got := Shape("resolve", raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("resolve changed its established product contract: %#v", got)
	}
}

func TestSchemaDescribeKeepsReportShapeSeparateFromRead(t *testing.T) {
	raw := map[string]any{"repository": "r", "commit": "c1", "schemas": []any{}, "reference": testReference, "locator": "/schema", "address": map[string]any{"objectId": "schema/order"}, "digest": "internal", "declarations": []any{"internal"}, "provenance": map[string]any{"actor": "internal"}}
	want := map[string]any{"repository": "r", "commit": "c1", "schemas": []any{}}
	if got := Shape("schema describe", raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("schema describe acquired unrelated read evidence fields: %#v", got)
	}
}

func TestCollectionProductViewKeepsEachUnitsOwnSource(t *testing.T) {
	declarations := []any{
		map[string]any{"address": map[string]any{"objectId": "table/orders", "aspectName": "columns", "memberKey": "id"}, "provenance": map[string]any{"originKind": "SOURCE", "sourceRefs": []any{"source/one"}}},
		map[string]any{"address": map[string]any{"objectId": "table/orders", "aspectName": "columns", "memberKey": "status"}, "provenance": map[string]any{"originKind": "DERIVATION", "sourceRefs": []any{"source/two"}, "inputWorkspaceVersionRef": "fixed-input", "algorithm": map[string]any{"codeHash": "v1"}}},
	}
	got := Shape("read", map[string]any{"reference": "urn:kc:object:table/orders#/columns", "repository": "r", "objectId": "table/orders", "commit": "fixed", "locator": "/columns", "value": map[string]any{"id": "one", "status": "two"}, "declarations": declarations}).(map[string]any)
	if !reflect.DeepEqual(got["declarations"], declarations) {
		t.Fatalf("CLI dropped or combined distinct recorded origins: %#v", got)
	}
}

func TestReferenceReadKeepsItsMaintenanceAndSourceEvidence(t *testing.T) {
	reference := "urn:kc:object:note/hello#/readme"
	address := map[string]any{"objectId": "note/hello", "aspectName": "readme", "kind": "Aspect"}
	declarations := []any{map[string]any{"schemaRef": "schema/readme"}}
	provenance := map[string]any{"sourceRefs": []any{"source/hello"}}
	raw := map[string]any{"reference": reference, "locator": "/readme", "repository": "r", "commit": "c1", "value": map[string]any{"body": "hello"}, "address": address, "digest": "d1", "declarations": declarations, "provenance": provenance}
	want := map[string]any{"reference": reference, "locator": "/readme", "repository": "r", "objectId": "note/hello", "commit": "c1", "aspectName": "readme", "schemaRef": "schema/readme", "value": map[string]any{"body": "hello"}, "address": address, "digest": "d1", "declarations": declarations, "provenance": provenance}
	if got := Shape("read", raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("reference read lost fixed maintenance or source evidence: %#v", got)
	}
}

func TestReadProductViewPreservesLogicalObservationBasis(t *testing.T) {
	observations := []any{map[string]any{"address": map[string]any{"objectId": "table/orders", "aspectName": "stats"}, "declarationCommit": "fixed", "declarationDigest": "schema-generation", "basis": map[string]any{"sourceRevision": "source-v2", "observedAt": "2026-10-13T02:00:00Z"}}}
	got := Shape("read", map[string]any{"reference": "urn:kc:object:table/orders#/stats", "locator": "/stats", "repository": "r", "commit": "fixed", "objectId": "table/orders", "value": map[string]any{"rowCount": 12}, "observations": observations}).(map[string]any)
	if !reflect.DeepEqual(got["observations"], observations) {
		t.Fatalf("logical value lost the basis distinguishing source observation from fixed declaration: %#v", got)
	}
}

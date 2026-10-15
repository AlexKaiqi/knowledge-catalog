package reader_test

import (
	"encoding/json"
	"reflect"
	"testing"

	objectmodel "kc/object"
	"kc/snapshot"
)

func TestReferenceReadRetainsEachSelectedUnitsRecordedProvenance(t *testing.T) {
	rd, guard, w, at := newReferenceReadFixture(t)
	source := &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, ActorRef: "source-owner", ActivityRef: "source-import", SourceRefs: []string{"source/one"}, EvidenceRefs: []string{"evidence/one"}, ProducedAt: "2026-10-13T01:00:00Z"}
	derivation := &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginDerivation, ActorRef: "deriver", ActivityRef: "derive-column", SourceRefs: []string{"source/two"}, EvidenceRefs: []string{"evidence/two"}, InputWorkspaceVersionRef: "ks://inputs@1", Algorithm: &objectmodel.AlgorithmRef{DerivationSpecRef: "algorithm/columns", ModelRef: "model/v1", CodeHash: "code-v1"}, ProducedAt: "2026-10-13T02:00:00Z"}
	for _, seed := range []struct {
		id         string
		member     string
		value      any
		provenance *objectmodel.ProvenanceEnvelope
	}{{"source-unit", "order_id", map[string]any{"type": "string", "constraints": map[string]any{"nullable": false}}, source}, {"derived-unit", "status", map[string]any{"type": "integer"}, derivation}} {
		receipt, err := w.Commit(seed.id, objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: seed.member}, Value: seed.value}}, Provenance: seed.provenance})
		if err != nil {
			t.Fatal(err)
		}
		at = receipt.Result.NewCommit
	}
	sources, err := w.Commit("actual-source-objects", objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "source/one"}, Value: "source content", PathHint: "sources/one.yaml"}, {Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "source/two"}, Value: "input content", PathHint: "sources/two.yaml"}}})
	if err != nil {
		t.Fatal(err)
	}
	at = sources.Result.NewCommit
	// Advancing HEAD must not replace the recorded source used at the fixed basis.
	_, err = w.Commit("advance-source-envelope", objectmodel.ChangeSet{TargetRepository: guard.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "status"}, Value: map[string]any{"type": "integer"}}}, Provenance: &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, SourceRefs: []string{"newer/source"}}})
	if err != nil {
		t.Fatal(err)
	}
	guard.blocked["sources/one.yaml"] = true
	guard.blocked["sources/two.yaml"] = true
	expected := map[string]any{"order_id": provenanceJSON(t, source), "status": provenanceJSON(t, derivation)}
	for _, locator := range []string{"", "/columns", "/columns/order_id", "/columns/status", "/columns/order_id/constraints/nullable"} {
		t.Run(locator, func(t *testing.T) {
			value, err := rd.ReadReference(objectmodel.Reference{Repository: guard.ID(), Object: "table/orders", Locator: locator}, at)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			found := map[string]any{}
			for _, raw := range response["declarations"].([]any) {
				declaration := raw.(map[string]any)
				address := declaration["address"].(map[string]any)
				member, _ := address["memberKey"].(string)
				if member != "" {
					found[member] = declaration["provenance"]
				}
			}
			want := expected
			if locator == "/columns/order_id" || locator == "/columns/order_id/constraints/nullable" {
				want = map[string]any{"order_id": expected["order_id"]}
			} else if locator == "/columns/status" {
				want = map[string]any{"status": expected["status"]}
			}
			if value.Commit != at || !reflect.DeepEqual(found, want) {
				t.Fatalf("selected data lost or replaced its fixed recorded sources: locator=%s got=%#v want=%#v", locator, found, want)
			}
		})
	}
	for _, path := range []string{"sources/one.yaml", "sources/two.yaml"} {
		if guard.reads[path] != 0 {
			t.Fatalf("recorded source was recursively read: %s %v", path, guard.reads)
		}
	}
}

func provenanceJSON(t *testing.T, value *objectmodel.ProvenanceEnvelope) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRecordedProvenanceDoesNotChangeSchemaDeclarationDigest(t *testing.T) {
	unit := objectmodel.Unit{ObjectID: "fixture/a", Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/a", AspectName: "x"}, SchemaRef: "schema/example@fixed", Value: "same"}
	before := objectmodel.DeclareUnits([]objectmodel.Unit{unit})[0]
	unit.Provenance = &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, SourceRefs: []string{"source/one"}}
	after := objectmodel.DeclareUnits([]objectmodel.Unit{unit})[0]
	if before.DeclarationDigest != after.DeclarationDigest || !reflect.DeepEqual(after.Provenance, unit.Provenance) {
		t.Fatalf("recorded origin changed Schema/ValueSource declaration identity: before=%#v after=%#v", before, after)
	}
}

package discovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type memberMemo struct {
	values      map[kernel.Digest][]byte
	misses      int
	fieldMisses int
}

func (m *memberMemo) ComputeMember(_ context.Context, key kernel.Digest, compute func() ([]byte, error)) ([]byte, error) {
	if value, ok := m.values[key]; ok {
		return bytes.Clone(value), nil
	}
	value, err := compute()
	if err != nil {
		return nil, err
	}
	var stage struct {
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal(value, &stage); err != nil {
		return nil, err
	}
	if stage.Stage == "member" {
		m.misses++
	} else {
		m.fieldMisses++
	}
	m.values[key] = bytes.Clone(value)
	return value, nil
}

func TestMemberComputationsReuseIndependentInputsAndConstructionVersions(t *testing.T) {
	repo, at := declarationBuildFixture(t, "skip")
	memo := &memberMemo{values: map[kernel.Digest][]byte{}}
	extract := func(commit kernel.CommitID) retrieval.FieldSet {
		t.Helper()
		spec, err := retrieval.AccessSpecForRepository(repo, commit)
		if err != nil {
			t.Fatal(err)
		}
		value, err := repo.Read("item/full", commit)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := retrieval.ExtractFields(repo, value, nil, spec)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := retrieval.ExtractFieldsWithComputations(context.Background(), repo, value, nil, spec, memo)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("memoized construction differs from exact interpretation: %#v expected=%#v", actual, expected)
		}
		return actual
	}
	apply := func(op objectmodel.Operation) kernel.CommitID {
		t.Helper()
		next, err := repo.(*testkit.ObjectRepository).ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{op}})
		if err != nil {
			t.Fatal(err)
		}
		at = next
		return next
	}
	extract(at)
	if memo.misses != 2 || memo.fieldMisses != 3 {
		t.Fatalf("initial member constructions=%d", memo.misses)
	}
	// Rewriting an unindexed value changes the source version, not either input.
	apply(objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/full"}, SchemaRef: "schema/item", Value: map[string]any{"name": "GMV", "note": "Discount", "unit": "CnY", "unindexed": "new source version"}})
	extract(at)
	if memo.misses != 2 || memo.fieldMisses != 3 {
		t.Fatalf("unchanged effective members recomputed after source rewrite: %d", memo.misses)
	}
	// Only the keyword member's input changes.
	apply(objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/full"}, SchemaRef: "schema/item", Value: map[string]any{"name": "GMV", "note": "Discount", "unit": "USD"}})
	extract(at)
	if memo.misses != 3 || memo.fieldMisses != 4 {
		t.Fatalf("changed one input recomputed unrelated member: %d", memo.misses)
	}
	member := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/item", AspectName: "index", MemberKey: "label"}
	raw, err := repo.ReadAddress(member, at)
	if err != nil {
		t.Fatal(err)
	}
	definition := raw.Value.(map[string]any)
	definition["build"].(map[string]any)["template"] = "New:{name}|{note}"
	apply(objectmodel.Operation{Op: objectmodel.OpPut, Address: member, SchemaRef: string(objectmodel.IndexDeclarationSchemaV1), Value: definition})
	if out := extract(at); out.Text != "New:GMV|Discount" || memo.misses != 4 {
		t.Fatalf("same member key hid changed construction: %#v misses=%d", out, memo.misses)
	}
}

func TestVectorOnlyMemberComputationNeedsNoLexicalFace(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/cached-vector")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	declaration := testkit.IndexMember("semantic", "vector", []string{"semantic"}, "schema/item#/fields/name")
	declaration.Value.(map[string]any)["build"].(map[string]any)["model"] = "model@1"
	at, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"name": map[string]any{"type": "string"}}}},
		declaration, {Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, SchemaRef: "schema/item", Value: map[string]any{"name": "vector input"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(repo, at)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("item/a", at)
	if err != nil {
		t.Fatal(err)
	}
	memo := &memberMemo{values: map[kernel.Digest][]byte{}}
	for n := 0; n < 2; n++ {
		out, err := retrieval.ExtractFieldsWithComputations(context.Background(), repo, value, nil, spec, memo)
		if err != nil || len(out.Semantic) != 1 || out.Semantic[0].Text != "vector input" || len(out.Values) != 0 || out.Text != "" {
			t.Fatalf("vector-only computation: %#v %v", out, err)
		}
	}
	if memo.misses != 1 {
		t.Fatalf("vector input computation misses=%d", memo.misses)
	}

	declaration.Value.(map[string]any)["build"].(map[string]any)["model"] = "model@2"
	next, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{declaration}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := retrieval.AccessSpecForRepository(repo, next)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repo.Read("item/a", next)
	if err != nil {
		t.Fatal(err)
	}
	out, err := retrieval.ExtractFieldsWithComputations(context.Background(), repo, current, nil, changed, memo)
	if err != nil || len(out.Semantic) != 1 || out.Semantic[0].Model != "model@2" || memo.misses != 2 || memo.fieldMisses != 1 {
		t.Fatalf("same member model change reused incompatible construction or extraction: %#v misses=%d fields=%d %v", out, memo.misses, memo.fieldMisses, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := retrieval.ExtractFieldsWithComputations(ctx, repo, value, nil, spec, memo); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled computation: %v", err)
	}
}

func TestMemberComputationSchemaMajorVersionOnlyInvalidatesRelatedOperations(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/member-schema-version")
	at := testkit.MustHead(t, repo, snapshot.DefaultRef)
	apply := func(operations ...objectmodel.Operation) {
		t.Helper()
		next, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: operations})
		if err != nil {
			t.Fatal(err)
		}
		at = next
	}
	schema := func(id, aspect string, fields map[string]any) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, Value: map[string]any{"entity": "Item", "aspect": aspect, "fields": fields}}
	}
	label := testkit.IndexMember("label", "text", []string{"text"}, "schema/label/v1#/fields/name", "schema/label/v1#/fields/note")
	label.Value.(map[string]any)["build"].(map[string]any)["template"] = "Label:{name}|{note}"
	apply(schema("schema/label/v1", "label", map[string]any{"name": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}}),
		schema("schema/quantity/v1", "quantity", map[string]any{"amount": map[string]any{"type": "string"}}), label,
		testkit.IndexMember("unit", "keyword", []string{"filter"}, "schema/quantity/v1#/fields/amount"),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/a", AspectName: "label"}, SchemaRef: "schema/label/v1", Value: map[string]any{"name": "GMV", "note": "Discount"}},
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/a", AspectName: "quantity"}, SchemaRef: "schema/quantity/v1", Value: map[string]any{"amount": "12"}})
	memo := &memberMemo{values: map[kernel.Digest][]byte{}}
	extract := func() {
		t.Helper()
		spec, err := retrieval.AccessSpecForRepository(repo, at)
		if err != nil {
			t.Fatal(err)
		}
		value, err := repo.Read("item/a", at)
		if err != nil {
			t.Fatal(err)
		}
		out, err := retrieval.ExtractFieldsWithComputations(context.Background(), repo, value, nil, spec, memo)
		if err != nil || out.Text != "Label:GMV|Discount" {
			t.Fatalf("changed Schema construction: %#v %v", out, err)
		}
	}
	extract()
	if memo.misses != 2 || memo.fieldMisses != 3 {
		t.Fatalf("initial computations: members=%d fields=%d", memo.misses, memo.fieldMisses)
	}
	apply(schema("schema/quantity/v2", "quantity", map[string]any{"amount": map[string]any{"type": "integer"}}),
		testkit.IndexMember("unit", "keyword", []string{"filter"}, "schema/quantity/v2#/fields/amount"),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/a", AspectName: "quantity"}, SchemaRef: "schema/quantity/v2", Value: map[string]any{"amount": 12}})
	extract()
	if memo.misses != 3 || memo.fieldMisses != 4 {
		t.Fatalf("Schema major release invalidated unrelated operations: members=%d fields=%d", memo.misses, memo.fieldMisses)
	}
}

func TestMemberComputationCacheCannotBypassCurrentObservationDeclaration(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/live", AspectName: "props"}
	source := bindingfixture.Source("resource/live")
	digest := objectmodel.BindingDeclarationDigest(objectmodel.DeclarationDigest("schema/live", source), "descriptor-v1")
	value := objectmodel.ObjectValue{Commit: "c1", Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/live"}, Value: map[string]any{"props": map[string]any{"owner": "alice"}}, Declarations: []objectmodel.UnitDeclaration{{Address: address, SchemaRef: "schema/live", ValueSource: source, DeclarationDigest: digest}}}
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/live", Aspect: "props", Path: "owner"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}}}}
	observation := objectmodel.UnitObservation{Address: address, DeclarationCommit: "c1", DeclarationDigest: digest}
	memo := &memberMemo{values: map[kernel.Digest][]byte{}}
	if _, err := retrieval.ExtractFieldsWithComputations(context.Background(), nil, value, []objectmodel.UnitObservation{observation}, spec, memo); err != nil {
		t.Fatal(err)
	}
	if memo.fieldMisses != 1 {
		t.Fatal("valid observation did not establish reusable extraction")
	}
	observation.DeclarationCommit = "another-source-version"
	if _, err := retrieval.ExtractFieldsWithComputations(context.Background(), nil, value, []objectmodel.UnitObservation{observation}, spec, memo); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("cached value bypassed mismatched observation declaration: %v", err)
	}
	if memo.fieldMisses != 1 {
		t.Fatal("invalid observation invoked member computation")
	}
}

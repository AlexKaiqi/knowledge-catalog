package discovery_test

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func declarationBuildFixture(t *testing.T, missing string) (objectmodel.Repository, kernel.CommitID) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://test/declaration-build")
	decl := func(key, kind, field string, build map[string]any, missing string) objectmodel.Operation {
		build["kind"] = kind
		uses := []any{"text"}
		if kind == "keyword" {
			uses = []any{"filter"}
		}
		input := map[string]any{"from": "schema/item#/fields/" + field}
		if missing != "" {
			input["missing"] = missing
		}
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/item", AspectName: "index", MemberKey: key}, SchemaRef: string(objectmodel.IndexDeclarationSchemaV1), Value: map[string]any{"catalog": "declarative-index/opensearch@2026-10", "inputs": []any{input}, "build": build, "uses": uses}}
	}
	text := decl("label", "text", "name", map[string]any{"template": "Label:{name}|{note}"}, "")
	body := text.Value.(map[string]any)
	input := map[string]any{"from": "schema/item#/fields/note"}
	if missing != "" {
		input["missing"] = missing
	}
	body["inputs"] = append(body["inputs"].([]any), input)
	head := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"name": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}, "unit": map[string]any{"type": "string"}}}},
		text, decl("unit", "keyword", "unit", map[string]any{"normalizer": "lowercase"}, ""),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/full"}, SchemaRef: "schema/item", Value: map[string]any{"name": "GMV", "note": "Discount", "unit": "CnY"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/missing"}, SchemaRef: "schema/item", Value: map[string]any{"name": "GMV", "unit": "CnY"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return repo, commit
}

func TestDeclaredBuildsConstructTemplateAndNormalizeOnlyKeyword(t *testing.T) {
	repo, commit := declarationBuildFixture(t, "skip")
	spec, err := retrieval.AccessSpecForRepository(repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.ReadAddress(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/full"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := retrieval.ExtractFields(repo, value, nil, spec)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Text != "Label:GMV|Discount" {
		t.Fatalf("template was not constructed: %q", doc.Text)
	}
	for _, cell := range doc.Values {
		if cell.Field.Path == "unit" && cell.Value != "cny" {
			t.Fatalf("keyword normalizer ignored: %#v", cell)
		}
	}
	resolved, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchEQ("unit", "CNY")), spec)
	if err != nil || resolved.Clauses[0].Value != "cny" {
		t.Fatalf("query normalizer ignored: %#v %v", resolved, err)
	}
	matched, err := retrieval.MatchFields(resolved, doc, spec)
	if err != nil || !matched {
		t.Fatalf("normalized query and build disagree: %v %v", matched, err)
	}
}

func TestDeclaredInputMissingPolicyActsPerMember(t *testing.T) {
	for _, missing := range []string{"skip", "exclude"} {
		t.Run(missing, func(t *testing.T) {
			repo, commit := declarationBuildFixture(t, missing)
			spec, err := retrieval.AccessSpecForRepository(repo, commit)
			if err != nil {
				t.Fatal(err)
			}
			value, err := repo.ReadAddress(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/missing"}, commit)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := retrieval.ExtractFields(repo, value, nil, spec)
			if err != nil {
				t.Fatal(err)
			}
			want := "Label:GMV|"
			if missing == "exclude" {
				want = ""
			}
			if doc.Text != want {
				t.Fatalf("%s policy: text=%q want=%q", missing, doc.Text, want)
			}
			if len(doc.Values) == 0 || !strings.Contains(doc.Values[len(doc.Values)-1].Value, "cny") {
				t.Fatalf("one missing member disabled another: %#v", doc.Values)
			}
		})
	}
}

func TestDeclaredNumericAndKeywordMembersRemainIndependent(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/declaration-ambiguity")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/count"}, Value: map[string]any{"entity": "Counter", "fields": map[string]any{"count": map[string]any{"type": "integer"}}}},
		testkit.IndexField("schema/count", "count", "number"), testkit.IndexField("schema/count", "count", "keyword"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "counter/a"}, SchemaRef: "schema/count", Value: map[string]any{"count": 6}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	_, err = retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchEQ("count", "6")), spec)
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), "keyword") || !strings.Contains(err.Error(), "number") {
		t.Fatalf("overlapping equality members were not reported: %v", err)
	}
	resolved, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGTE, "count", "5")), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolved.Clauses[0].Member, "number") {
		t.Fatalf("range selected another construction: %#v", resolved)
	}
	value, err := repo.Read("counter/a", commit)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := retrieval.ExtractFields(repo, value, nil, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Values) != 2 || doc.Values[0].Key() == doc.Values[1].Key() {
		t.Fatalf("parallel members shared a physical field: %#v", doc.Values)
	}
	matched, err := retrieval.MatchFields(resolved, doc, spec)
	if err != nil || !matched {
		t.Fatalf("numeric member was not independently usable: %v %v", matched, err)
	}
}

func TestComposedBindingObservationUsesItsSourceCommit(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "item/a", AspectName: "health"}
	unit := objectmodel.UnitDeclaration{Address: address, SchemaRef: "schema/health", DeclarationDigest: "decl", ValueSource: bindingfixture.Source("resource/health")}
	value := objectmodel.ObjectValue{Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, Value: map[string]any{"health": map[string]any{"state": "ready"}}, Declarations: []objectmodel.UnitDeclaration{unit}, Sources: []objectmodel.SourceBasis{{Repository: "kr://props", Commit: "props-c1", Aspects: []string{"properties"}}, {Repository: "kr://health", Commit: "health-c2", Aspects: []string{"health"}}}}
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/health", Aspect: "health", Path: "state"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}}}}
	observation := objectmodel.UnitObservation{Address: address, DeclarationCommit: "health-c2", DeclarationDigest: "decl"}
	doc, err := retrieval.ExtractFields(nil, value, []objectmodel.UnitObservation{observation}, spec)
	if err != nil || len(doc.Values) != 1 {
		t.Fatalf("composed source observation was rejected: %#v %v", doc, err)
	}
	observation.DeclarationCommit = "props-c1"
	if _, err := retrieval.ExtractFields(nil, value, []objectmodel.UnitObservation{observation}, spec); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("another aspect's basis was accepted: %v", err)
	}
}

func TestVectorMembersBuildIndependentInputsWithoutLexicalAccess(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/vector-members")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	first := testkit.IndexMember("first", "vector", []string{"semantic"}, "schema/item#/fields/name")
	second := testkit.IndexMember("second", "vector", []string{"semantic"}, "schema/item#/fields/note")
	for _, op := range []objectmodel.Operation{first, second} {
		op.Value.(map[string]any)["build"].(map[string]any)["model"] = "embed/test-hash@1"
	}
	first.Value.(map[string]any)["build"].(map[string]any)["template"] = "Label:{name}"
	commit, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"name": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}}}},
		first, second,
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, SchemaRef: "schema/item", Value: map[string]any{"name": "GMV", "note": "Discount"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("item/a", commit)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := retrieval.ExtractFields(repo, value, nil, spec)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Text != "" || len(doc.Values) != 0 || len(spec.Fields) != 0 {
		t.Fatalf("semantic members supplied undeclared lexical access: %#v %#v", doc, spec.Fields)
	}
	if len(doc.Semantic) != 2 || doc.Semantic[0].Text != "Label:GMV" || doc.Semantic[1].Text != "Discount" || doc.Semantic[0].Member == doc.Semantic[1].Member {
		t.Fatalf("vector inputs lost member construction: %#v", doc.Semantic)
	}
}

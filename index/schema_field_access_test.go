package index

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
	"kc/snapshot"
)

func TestSchemaFieldAccessIsTheOnlyIndexDeclarationSource(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/schema-field-access")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	first, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"name": map[string]any{"type": "string", "access": []any{"text"}}, "secret": map[string]any{"type": "string", "access": []any{"filter"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: first, ExpectedTargetCommit: first, Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "index/items"}, Value: map[string]any{"targets": []any{map[string]any{"schemaRef": "schema/item", "fields": map[string]any{"name": []any{"filter"}}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	old, err := specAtCommit(repo, first)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := specAtCommit(repo, ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Fields) != 2 || spec.AccessDigest != old.AccessDigest {
		t.Fatalf("ordinary index/* objects must not supply query declarations: old=%#v new=%#v", old, spec)
	}
	current, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: ordinary, ExpectedTargetCommit: ordinary,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/item"},
			Value: map[string]any{"entity": "Item", "fields": map[string]any{
				"name":   map[string]any{"type": "string", "access": []any{"filter"}},
				"secret": map[string]any{"type": "string"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err = specAtCommit(repo, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Fields) != 1 || spec.Fields[0].Path != "name" || !spec.Fields[0].Has(reader.HintFilter) || spec.Fields[0].Has(reader.HintText) {
		t.Fatalf("field access must supply the current query contract: %#v", spec.Fields)
	}
	old, err = specAtCommit(repo, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Fields) != 2 || old.AccessDigest == spec.AccessDigest {
		t.Fatalf("historical contract changed: old=%#v new=%#v", old, spec)
	}
}

func TestSchemaFieldAccessChangeRebuildsProjectionAndContentRemainsIncremental(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/index-rebuild")
	commit := func(base kernel.CommitID, ops ...knowledge.Operation) kernel.CommitID {
		t.Helper()
		next, err := repo.ApplyKnowledgeCommit(knowledge.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	putSchema := func(hints ...string) knowledge.Operation {
		return knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": hints}}}}
	}
	article := func(text string) knowledge.Operation {
		return knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "article/a"}, SchemaRef: "schema/article", Value: map[string]any{"body": text}}
	}
	first := commit(testkit.MustHead(t, repo, snapshot.DefaultRef), putSchema("text"), article("before"))
	engine := &stateTestEngine{}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (Engine, error) { return engine, nil })
	if _, err := idx.Ensure(repo, first); err != nil {
		t.Fatal(err)
	}
	second := commit(first, putSchema("filter"))
	result, err := idx.Ensure(repo, second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != IndexModeRebuild || engine.docs["article/a"].Cells[0].StringValue == nil || engine.docs["article/a"].Cells[0].TextValue != "" {
		t.Fatalf("declaration changed without reinterpreting documents: %#v %#v", result, engine.docs["article/a"])
	}
	third := commit(second, article("after"))
	result, err = idx.Ensure(repo, third)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != IndexModeIncremental || engine.docs["article/a"].Cells[0].Value != "after" {
		t.Fatalf("content edit unnecessarily rebuilt or stayed stale: %#v", result)
	}
	fourth := commit(third, putSchema())
	result, err = idx.Ensure(repo, fourth)
	if err != nil {
		t.Fatal(err)
	}
	doc := engine.docs["article/a"]
	if result.Mode != IndexModeRebuild || len(doc.Cells) != 0 || len(doc.EligibleFields) != 0 || doc.Text != "" {
		t.Fatalf("removing all field access must rebuild and clear stale query fields: %#v %#v", result, doc)
	}
	_, err = idx.SearchAt(repo, fourth, retrieval.SearchOf(retrieval.SearchClause{
		Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Value: "after",
	}))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("removed field must no longer be queryable: %v", err)
	}
}

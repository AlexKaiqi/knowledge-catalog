package projection

import (
	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestIndexDeclarationMembersAreTheOnlyRetrievalSource(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/schema-field-access")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	nameText, secretFilter, nameFilter := testkit.IndexText("schema/item", "name"), testkit.IndexField("schema/item", "secret", "keyword"), testkit.IndexField("schema/item", "name", "keyword")
	first, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"name": map[string]any{"type": "string"}, "secret": map[string]any{"type": "string"}}}}, nameText, secretFilter}})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: first, ExpectedTargetCommit: first, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "index/items"}, Value: map[string]any{"targets": []any{map[string]any{"schemaRef": "schema/item", "fields": map[string]any{"name": []any{"filter"}}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	old, err := (&Index{}).specAtCommit(repo, first)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := (&Index{}).specAtCommit(repo, ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Fields) != 2 || spec.AccessDigest != old.AccessDigest {
		t.Fatalf("ordinary index/* objects must not supply query declarations: old=%#v new=%#v", old, spec)
	}
	current, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: ordinary, ExpectedTargetCommit: ordinary,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpRemove, Address: nameText.Address},
			{Op: objectmodel.OpRemove, Address: secretFilter.Address},
			nameFilter,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err = (&Index{}).specAtCommit(repo, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Fields) != 1 || spec.Fields[0].Path != "name" || !spec.Fields[0].Has(reader.HintFilter) || spec.Fields[0].Has(reader.HintText) {
		t.Fatalf("declaration members must supply the current query contract: %#v", spec.Fields)
	}
	old, err = (&Index{}).specAtCommit(repo, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Fields) != 2 || old.AccessDigest == spec.AccessDigest {
		t.Fatalf("historical contract changed: old=%#v new=%#v", old, spec)
	}
}

func TestIndexDeclarationChangeRebuildsProjectionAndContentRemainsIncremental(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/index-rebuild")
	commit := func(base kernel.CommitID, ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		next, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	schema := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}
	text, filter := testkit.IndexText("schema/article", "body"), testkit.IndexField("schema/article", "body", "keyword")
	remove := func(op objectmodel.Operation) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpRemove, Address: op.Address}
	}
	article := func(text string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/a"}, SchemaRef: "schema/article", Value: map[string]any{"body": text}}
	}
	first := commit(testkit.MustHead(t, repo, snapshot.DefaultRef), schema, text, article("before"))
	engine := &testfixture.StateEngine{}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	if _, err := idx.Ensure(repo, first); err != nil {
		t.Fatal(err)
	}
	second := commit(first, remove(text), filter)
	result, err := idx.Ensure(repo, second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != ports.IndexModeRebuild || engine.Docs["article/a"].Cells[0].StringValue == nil || engine.Docs["article/a"].Cells[0].TextValue != "" {
		t.Fatalf("declaration changed without reinterpreting documents: %#v %#v", result, engine.Docs["article/a"])
	}
	third := commit(second, article("after"))
	result, err = idx.Ensure(repo, third)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != ports.IndexModeIncremental || engine.Docs["article/a"].Cells[0].Value != "after" {
		t.Fatalf("content edit unnecessarily rebuilt or stayed stale: %#v", result)
	}
	fourth := commit(third, remove(filter))
	result, err = idx.Ensure(repo, fourth)
	if err != nil {
		t.Fatal(err)
	}
	doc := engine.Docs["article/a"]
	if result.Mode != ports.IndexModeRebuild || len(doc.Cells) != 0 || len(doc.EligibleFields) != 0 || doc.Text != "" {
		t.Fatalf("removing every declaration member must rebuild and clear stale query fields: %#v %#v", result, doc)
	}
	_, err = idx.SearchAt(repo, fourth, retrieval.SearchOf(retrieval.SearchClause{
		Op: retrieval.OpEQ, Field: &retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Value: "after",
	}))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("removed field must no longer be queryable: %v", err)
	}
}

package writer_test

import (
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// Retrieval capability lives in index declaration members, so narrowing or
// removing it is a member change that leaves the Schema value structure alone.
func TestIndexDeclarationCanNarrowAndDisappearWithoutChangingValueStructure(t *testing.T) {
	text := testkit.IndexText("schema/article", "body")
	filter := testkit.IndexField("schema/article", "body", "keyword")
	for _, tc := range []struct {
		name    string
		removed []objectmodel.Operation
	}{
		{name: "narrowed", removed: []objectmodel.Operation{text}},
		{name: "removed", removed: []objectmodel.Operation{text, filter}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			initial, err := s.Writer.Commit("index-declaration", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: articleSchema()},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/a"}, SchemaRef: "schema/article", Value: map[string]any{"body": "fixed content"}},
				text, filter,
			}})
			if err != nil {
				t.Fatal(err)
			}
			deletes := make([]objectmodel.Operation, len(tc.removed))
			for i, op := range tc.removed {
				deletes[i] = objectmodel.Operation{Op: objectmodel.OpRemove, Address: op.Address}
			}
			next, err := s.Writer.Commit("change-declaration", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: initial.Result.CommitID, ExpectedTargetCommit: initial.Result.CommitID, Operations: deletes})
			if err != nil {
				t.Fatalf("index declaration is independently editable from the value structure: %v", err)
			}
			before := declaredAccess(t, s.Repo, initial.Result.CommitID)
			after := declaredAccess(t, s.Repo, next.Result.CommitID)
			body := retrieval.FieldRef{Schema: "schema/article", Path: "body"}
			if field, err := before.ResolveField(body); err != nil || !field.Has(reader.HintText) || !field.Has(reader.HintFilter) {
				t.Fatalf("old pin lost its declared retrieval: %#v %v", before.Fields, err)
			}
			if len(tc.removed) == 2 && len(after.Fields) != 0 {
				t.Fatalf("removed declaration still compiled: %#v", after.Fields)
			}
			if len(tc.removed) == 1 {
				if field, err := after.ResolveField(body); err != nil || field.Has(reader.HintText) || !field.Has(reader.HintFilter) {
					t.Fatalf("narrowed declaration not compiled: %#v %v", after.Fields, err)
				}
			}
			if before.AccessDigest == after.AccessDigest {
				t.Fatal("declaration change must invalidate the logical projection basis")
			}
		})
	}
}

// Only members adopting the trusted index declaration Schema declare
// retrieval; ordinary objects merely named index/* carry no capability.
func TestIndexDeclarationCompilerIgnoresOrdinaryIndexNamedObjects(t *testing.T) {
	s := testkit.NewSetup(t, "")
	basis, err := s.Repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: articleSchema()},
		testkit.IndexText("schema/article", "body"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "index/ordinary-data"}, Value: map[string]any{"targets": []any{map[string]any{"schemaRef": "schema/article", "fields": map[string]any{"body": []any{"filter"}}}}}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "index/notes"}, Value: map[string]any{"body": "ordinary object"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	spec := declaredAccess(t, s.Repo, basis)
	field, err := spec.ResolveField(retrieval.FieldRef{Schema: "schema/article", Path: "body"})
	if err != nil || len(spec.Fields) != 1 || !field.Has(reader.HintText) || field.Has(reader.HintFilter) {
		t.Fatalf("ordinary index/* data changed declared retrieval: %#v %v", spec.Fields, err)
	}
}

func TestIndexDeclarationDoesNotPermitStructuralBreaks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous map[string]any
		next     map[string]any
	}{
		{name: "type", previous: articleSchema(), next: map[string]any{"body": map[string]any{"type": "integer"}}},
		{name: "removed field", previous: articleSchema(), next: map[string]any{}},
		{name: "became required", previous: articleSchema(), next: map[string]any{"body": map[string]any{"type": "string", "required": true}}},
		{name: "added required", previous: articleSchema(), next: map[string]any{"body": map[string]any{"type": "string"}, "owner": map[string]any{"type": "string", "required": true}}},
		{name: "narrowed refTypes", previous: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "object_ref", "refTypes": []any{"table", "view"}}}}, next: map[string]any{"body": map[string]any{"type": "object_ref", "refTypes": []any{"table"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			kind := "text"
			if tc.name == "narrowed refTypes" {
				kind = "keyword"
			}
			declared := testkit.IndexMember("article-body", kind, []string{map[string]string{"text": objectmodel.IndexUseText, "keyword": objectmodel.IndexUseFilter}[kind]}, "schema/article#/fields/body")
			initial, err := s.Repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: tc.previous}, declared}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Writer.Commit("structure-break", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: initial, ExpectedTargetCommit: initial, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": tc.next}}}})
			testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
			if testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != initial {
				t.Fatal("structural break advanced Canonical")
			}
		})
	}
}

func articleSchema() map[string]any {
	return map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}
}

func declaredAccess(t *testing.T, repo objectmodel.Repository, basis kernel.CommitID) retrieval.AccessSpec {
	t.Helper()
	spec, err := retrieval.AccessSpecForRepository(repo, basis)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

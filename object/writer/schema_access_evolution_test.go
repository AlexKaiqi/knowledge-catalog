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

func TestSchemaAccessCanNarrowAndDisappearWithoutChangingValueStructure(t *testing.T) {
	for _, hints := range [][]any{{"filter"}, {}} {
		s := testkit.NewSetup(t, "")
		initial, err := s.Writer.Commit("schema-access", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: articleAccessSchema([]any{"text", "filter"})},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/a"}, SchemaRef: "schema/article", Value: map[string]any{"body": "fixed content"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		next, err := s.Writer.Commit("change-access", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: initial.Result.CommitID, ExpectedTargetCommit: initial.Result.CommitID, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: articleAccessSchema(hints)}}})
		if err != nil {
			t.Fatalf("query access is independently editable from the value structure: %v", err)
		}
		before := schemaAccessPlan(t, s.Repo, initial.Result.CommitID)
		after := schemaAccessPlan(t, s.Repo, next.Result.CommitID)
		if len(before.Fields) != 1 || !before.Fields[0].Has(reader.HintText) || !before.Fields[0].Has(reader.HintFilter) {
			t.Fatalf("old pin lost its contract: %#v", before)
		}
		if len(hints) == 0 && len(after.Fields) != 0 {
			t.Fatalf("removed access still compiled: %#v", after.Fields)
		}
		if len(hints) > 0 && (len(after.Fields) != 1 || after.Fields[0].Has(reader.HintText) || !after.Fields[0].Has(reader.HintFilter)) {
			t.Fatalf("narrowed contract not compiled: %#v", after.Fields)
		}
		if before.AccessDigest == after.AccessDigest {
			t.Fatal("access change must invalidate the logical projection basis")
		}
	}
}

func TestSchemaAccessCompilerIgnoresOrdinaryIndexNamedObjects(t *testing.T) {
	s := testkit.NewSetup(t, "")
	basis, err := s.Repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: articleAccessSchema([]any{"text"})},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "index/ordinary-data"}, Value: map[string]any{"targets": []any{map[string]any{"schemaRef": "schema/article", "fields": map[string]any{"body": []any{"filter"}}}}}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "index/notes"}, Value: map[string]any{"body": "ordinary object"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	spec := schemaAccessPlan(t, s.Repo, basis)
	if len(spec.Fields) != 1 || !spec.Fields[0].Has(reader.HintText) || spec.Fields[0].Has(reader.HintFilter) {
		t.Fatalf("ordinary index/* data changed Schema access: %#v", spec.Fields)
	}
}

func TestSchemaAccessChangesDoNotPermitStructuralBreaks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous map[string]any
		next     map[string]any
	}{
		{name: "type", previous: articleAccessSchema([]any{"text"}), next: map[string]any{"body": map[string]any{"type": "integer"}}},
		{name: "removed field", previous: articleAccessSchema([]any{"text"}), next: map[string]any{}},
		{name: "became required", previous: articleAccessSchema([]any{"text"}), next: map[string]any{"body": map[string]any{"type": "string", "required": true}}},
		{name: "added required", previous: articleAccessSchema([]any{"text"}), next: map[string]any{"body": map[string]any{"type": "string"}, "owner": map[string]any{"type": "string", "required": true}}},
		{name: "narrowed refTypes", previous: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "object_ref", "refTypes": []any{"table", "view"}, "access": []any{"filter"}}}}, next: map[string]any{"body": map[string]any{"type": "object_ref", "refTypes": []any{"table"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			initial, err := s.Repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: tc.previous}}})
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

func articleAccessSchema(hints []any) map[string]any {
	return map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": hints}}}
}

func schemaAccessPlan(t *testing.T, repo objectmodel.Repository, basis kernel.CommitID) retrieval.AccessSpec {
	t.Helper()
	pins := map[kernel.RepositoryID]kernel.CommitID{repo.ID(): basis}
	plan, err := retrieval.PlanAccess(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{Repositories: pins, Items: reader.WholeRepositoryItems(pins)})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Specs) != 1 {
		t.Fatalf("missing repository access contract: %#v", plan)
	}
	return plan.Specs[0]
}

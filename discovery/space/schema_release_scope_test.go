package space_test

import (
	"testing"

	retrieval "kc/discovery"
	"kc/discovery/space"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestDatasetDeclarationCannotBorrowUnselectedSchemaRelease(t *testing.T) {
	for _, qualifier := range []string{"", "kc://space/schema-owner/"} {
		t.Run(qualifier, func(t *testing.T) {
			repo := testkit.MakeRepository(t, "kr://space/schema-owner")
			base := testkit.MustHead(t, repo, snapshot.DefaultRef)
			old, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/private"}, PathHint: "private/schema.yaml", Value: map[string]any{"entity": "Private", "fields": map[string]any{"secret": map[string]any{"type": "string"}}}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			from := "schema/private@" + string(old) + "#/fields/secret"
			if qualifier != "" {
				from = "kc://space/schema-owner@" + string(old) + "/schema/private#/fields/secret"
			}
			at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: old, ExpectedTargetCommit: old, Operations: []objectmodel.Operation{
				testkit.IndexUnder("selected", testkit.IndexMember("private-secret", "text", []string{objectmodel.IndexUseText}, from)),
			}})
			if err != nil {
				t.Fatal(err)
			}
			pin := reader.DatasetPin{SetID: "restricted-schema", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: "selected"}}}
			view, err := space.Member(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, pin, repo.ID())
			if err != nil {
				t.Fatal(err)
			}
			spec, err := retrieval.AccessSpecForRepository(view, at)
			if err != nil || len(spec.Statuses()) != 1 || spec.Statuses()[0].Status != retrieval.DeclarationUnsupported || len(spec.Fields) != 0 {
				t.Fatalf("historical input borrowed an unselected Schema: %#v %v", spec, err)
			}
		})
	}
}

func TestDatasetDeclarationKeepsQualifiedSchemaAuthority(t *testing.T) {
	s := testkit.NewSetup(t, "kr://space/schema-owner")
	owner := s.Repo
	other := testkit.MakeRepository(t, "kr://space/schema-other")
	outside := testkit.MakeRepository(t, "kr://space/schema-outside")
	schema := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, PathHint: "schemas/article.yaml", Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}
	for _, repo := range []*testkit.ObjectRepository{other, outside} {
		if err := s.Store.Add(repo); err != nil {
			t.Fatal(err)
		}
		base := testkit.MustHead(t, repo, snapshot.DefaultRef)
		if _, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{schema}}); err != nil {
			t.Fatal(err)
		}
	}
	base := testkit.MustHead(t, owner, snapshot.DefaultRef)
	receipt, err := s.Writer.Commit("qualified-authority", objectmodel.CommitChangeSet{TargetRepository: owner.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		schema,
		testkit.IndexMember("valid", "text", []string{objectmodel.IndexUseText}, "kc://space/schema-owner/schema/article#/fields/body"),
		testkit.IndexMember("wrong-holder", "text", []string{objectmodel.IndexUseText}, "kc://space/schema-other/schema/article#/fields/body"),
		testkit.IndexMember("not-selected", "text", []string{objectmodel.IndexUseText}, "kc://space/schema-outside/schema/article#/fields/body"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	at := receipt.Result.NewCommit
	otherBase := testkit.MustHead(t, other, snapshot.DefaultRef)
	otherAt, err := other.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: other.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: otherBase, ExpectedTargetCommit: otherBase, Operations: []objectmodel.Operation{{Op: objectmodel.OpRemove, Address: schema.Address}}})
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{SetID: "qualified-authority", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{owner.ID(): at, other.ID(): otherAt}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		switch id {
		case owner.ID():
			return owner, nil
		case other.ID():
			return other, nil
		default:
			t.Fatalf("declaration escaped the selected repository set: %s", id)
			return nil, nil
		}
	}
	view, err := space.Member(lookup, pin, other.ID())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(view, otherAt)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"valid": retrieval.DeclarationReady, "wrong-holder": retrieval.DeclarationUnsupported, "not-selected": retrieval.DeclarationUnsupported}
	for _, status := range spec.Statuses() {
		if status.Status != want[status.Member.MemberKey] {
			t.Fatalf("qualified declaration changed Schema authority: %#v", status)
		}
		delete(want, status.Member.MemberKey)
	}
	if len(want) != 0 {
		t.Fatalf("missing declaration statuses: %v", want)
	}
}

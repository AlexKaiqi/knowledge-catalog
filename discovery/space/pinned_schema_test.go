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

func TestDatasetDeclarationKeepsPinnedSchemaRelease(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://space/pinned-schema")
	commit := func(operations ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		base, err := repo.Head(snapshot.DefaultRef)
		if err != nil {
			t.Fatal(err)
		}
		at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	schema := objectmodel.ObjectID("schema/article")
	old := commit(objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schema}, PathHint: "schemas/article.yaml", Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}})
	at := commit(
		testkit.IndexMember("pinned-body", "text", []string{objectmodel.IndexUseText}, string(schema)+"@"+string(old)+"#/fields/body"),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, PathHint: "objects/one.yaml", SchemaRef: string(schema) + "@" + string(old), Value: map[string]any{"body": "frozen article"}},
	)
	checkReady := func(repository objectmodel.Repository) {
		t.Helper()
		spec, err := retrieval.AccessSpecForRepository(repository, at)
		if err != nil {
			t.Fatal(err)
		}
		if len(spec.Statuses()) != 1 || spec.Statuses()[0].Status != retrieval.DeclarationReady {
			t.Fatalf("pinned Schema release was not retained: %#v", spec.Statuses())
		}
		retained, err := retrieval.AccessSpecFromAdoptions(repository, at, spec.AdoptionWitnesses())
		if err != nil || retained.AccessDigest != spec.AccessDigest {
			t.Fatalf("retained pinned declaration changed: %#v %v", retained, err)
		}
		value, err := repository.Read("article/one", at)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := retrieval.ExtractFields(repository, value, nil, retained)
		if err != nil || fields.Text != "frozen article" {
			t.Fatalf("pinned object lost its declaration's field: %#v %v", fields, err)
		}
	}
	checkReady(repo)
	pin := reader.DatasetPin{SetID: "pinned-schema", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "prefix"}}}
	view, err := space.Member(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, pin, repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	checkReady(view)
	if _, err := view.Read(schema, old); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("declaration interpretation opened arbitrary historical reads: %v", err)
	}
}

func TestDatasetDeclarationResolvesSelectedRepositoryQualifiedSchema(t *testing.T) {
	first := testkit.MakeRepository(t, "kr://space/a-source")
	owner := testkit.MakeRepository(t, "kr://space/z-schema")
	schema := objectmodel.ObjectID("schema/article")
	base, err := owner.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	at, err := owner.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: owner.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schema}, PathHint: "schemas/article.yaml", Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexMember("qualified-body", "text", []string{objectmodel.IndexUseText}, "kc://space/z-schema/schema/article#/fields/body"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	firstAt, err := first.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{SetID: "qualified-schema", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{first.ID(): firstAt, owner.ID(): at}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id == first.ID() {
			return first, nil
		}
		return owner, nil
	}
	for _, id := range []kernel.RepositoryID{owner.ID(), first.ID()} {
		view, err := space.Member(lookup, pin, id)
		if err != nil {
			t.Fatal(err)
		}
		spec, err := retrieval.AccessSpecForRepository(view, pin.Repositories[id])
		if err != nil {
			t.Fatal(err)
		}
		if len(spec.Statuses()) != 1 || spec.Statuses()[0].Status != retrieval.DeclarationReady {
			t.Fatalf("selected qualified Schema depended on leading member %s: %#v", id, spec.Statuses())
		}
	}
}

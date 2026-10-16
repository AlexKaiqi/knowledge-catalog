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

func TestDatasetDeclarationNamespaceCrossesSelectedSourcesOnly(t *testing.T) {
	source := testkit.MakeRepository(t, "kr://space/source")
	maintainer := testkit.MakeRepository(t, "kr://space/maintainer")
	schema := objectmodel.ObjectID("schema/article")
	schemaOp := objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schema}, PathHint: "schemas/article.yaml", Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}
	commit := func(repo *testkit.ObjectRepository, ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		base, _ := repo.Head(snapshot.DefaultRef)
		at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	a := commit(source, schemaOp, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, PathHint: "objects/one.yaml", SchemaRef: string(schema), Value: map[string]any{"body": "runbook"}})
	b := commit(maintainer, schemaOp, testkit.IndexUnder("indexes", testkit.IndexText(schema, "body")))
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id == source.ID() {
			return source, nil
		}
		return maintainer, nil
	}
	pin := reader.DatasetPin{SetID: "reading", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{source.ID(): a, maintainer.ID(): b}, Items: []reader.DatasetItem{
		{Repository: source.ID(), Commit: a, Kind: "prefix"},
		{Repository: maintainer.ID(), Commit: b, Kind: "prefix", Prefix: "indexes"},
	}}
	member, err := space.Member(lookup, pin, source.ID())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := retrieval.AccessSpecForRepository(member, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Statuses()) != 1 || spec.Statuses()[0].Status != retrieval.DeclarationReady {
		t.Fatalf("selected remote declaration did not compile: %#v", spec.Statuses())
	}
	if _, err := retrieval.ResolveSearch(retrieval.SearchRequest{Clauses: []retrieval.SearchClause{{Op: retrieval.OpMatch, Value: "runbook"}}}, spec); err != nil {
		t.Fatal(err)
	}
	plain := pin
	plain.SetID = "plain"
	plain.Items = append([]reader.DatasetItem(nil), pin.Items[:1]...)
	unselected, err := space.Member(lookup, plain, source.ID())
	if err != nil {
		t.Fatal(err)
	}
	plainSpec, err := retrieval.AccessSpecForRepository(unselected, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(plainSpec.Statuses()) != 0 {
		t.Fatalf("unselected declaration leaked: %#v", plainSpec.Statuses())
	}
	if _, err := retrieval.ResolveSearch(retrieval.SearchRequest{Clauses: []retrieval.SearchClause{{Op: retrieval.OpMatch, Value: "runbook"}}}, plainSpec); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unselected declaration gave search: %v", err)
	}
	if member.(objectmodel.StoreIdentified).StoreDigest() == unselected.(objectmodel.StoreIdentified).StoreDigest() {
		t.Fatal("distinct Dataset spaces shared a physical identity")
	}
	maintainerView, err := space.Member(lookup, pin, maintainer.ID())
	if err != nil {
		t.Fatal(err)
	}
	single, err := maintainerView.Read(schema, b)
	if err != nil {
		t.Fatal(err)
	}
	batched, err := maintainerView.(objectmodel.BatchReadStore).ReadMany([]objectmodel.ObjectID{schema}, b)
	if err != nil || batched[schema].Repository != source.ID() || batched[schema].Commit != a || kernel.CanonicalDigest(batched[schema].Value) != kernel.CanonicalDigest(single.Value) {
		t.Fatalf("batch read lost the selected cross-source Schema namespace: %#v %v", batched, err)
	}
}

func TestDatasetBatchReadMatchesSelectedPartialObject(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://space/partial-batch")
	base, _ := repo.Head(snapshot.DefaultRef)
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: "public"}, PathHint: "selected/public.yaml", Value: map[string]any{"name": "allowed"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: "private"}, PathHint: "private/secret.yaml", Value: map[string]any{"name": "forbidden"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/private"}, PathHint: "private/note.yaml", Value: map[string]any{"name": "outside"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo.Repository, nil }
	pin := reader.DatasetPin{SetID: "partial-batch", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: "selected"}}}
	view, err := space.Member(lookup, pin, repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	single, err := view.Read("article/one", at)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := view.(objectmodel.BatchReadStore).ReadMany([]objectmodel.ObjectID{"article/one", "note/private"}, at)
	if err != nil || len(batch) != 1 || kernel.CanonicalDigest(batch["article/one"].Value) != kernel.CanonicalDigest(single.Value) {
		t.Fatalf("partial object batch departed from selected point read: %#v %v", batch, err)
	}
	if _, leaked := batch["article/one"].Value.(map[string]any)["private"]; leaked {
		t.Fatalf("batch read exposed an unselected sibling: %#v", batch)
	}
}

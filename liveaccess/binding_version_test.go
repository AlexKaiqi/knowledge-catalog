package liveaccess_test

import (
	"kc/internal/testkit"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
	"testing"
)

func TestBindingDeclarationChangeIsVersionedWhenValueIsUnchanged(t *testing.T) {
	s := testkit.NewSetup(t, "")
	schema := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.health"}
	entity := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "properties"}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	schemaValue := func(origin string) map[string]any {
		return map[string]any{
			"entity": "Service", "aspect": "health", "origin": origin,
			"fields": map[string]any{"status": map[string]any{"type": "string"}},
		}
	}
	c1, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: schema, Value: schemaValue("https://stats.example/v1")},
			{Op: objectmodel.OpPut, Address: entity, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	b1, err := liveaccess.ResolveRepoBinding(s.Repo, c1, address)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: schema, Value: schemaValue("https://stats.example/v2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := liveaccess.ResolveRepoBinding(s.Repo, c2, address)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := s.Repo.ResolveAddress(entity, c1)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.Repo.ResolveAddress(entity, c2)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Digest != r2.Digest {
		t.Fatalf("entity Snapshot digest must stay stable: r1=%#v r2=%#v", r1, r2)
	}
	if b1.DeclarationDigest == b2.DeclarationDigest || b1.Origin == b2.Origin {
		t.Fatalf("schema origin change must version the Binding declaration: b1=%#v b2=%#v", b1, b2)
	}
	history, err := s.Repo.Log(schema.ObjectID, c2, objectmodel.ObjectLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 {
		t.Fatalf("LOG must retain Schema origin revisions: %#v", history)
	}
}

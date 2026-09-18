package knowledge_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func TestBindingDeclarationChangeIsVersionedWhenValueIsUnchanged(t *testing.T) {
	s := testkit.NewSetup(t, "")
	schema := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/service.health"}
	entity := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "properties"}
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	schemaValue := func(origin string) map[string]any {
		return map[string]any{
			"entity": "Service", "aspect": "health", "origin": origin,
			"fields": map[string]any{"status": map[string]any{"type": "string"}},
		}
	}
	c1, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: schema, Value: schemaValue("https://stats.example/v1")},
			{Op: knowledge.OpPut, Address: entity, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	b1, err := s.Reader.ResolveBinding(s.RepositoryID, c1, address)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: schema, Value: schemaValue("https://stats.example/v2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := s.Reader.ResolveBinding(s.RepositoryID, c2, address)
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
	history, err := s.Repo.Log(schema.ObjectID, c2, knowledge.ObjectLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 {
		t.Fatalf("LOG must retain Schema origin revisions: %#v", history)
	}
}

func TestValidateBindingRejectsAmbiguousAndIncompleteDeclarations(t *testing.T) {
	cases := []*knowledge.ValueSource{
		{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState}},
		{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: "snapshot", Runtime: "r", Protocol: "mcp", Operations: map[string]knowledge.BindingOperation{"read": {Call: "x"}}}},
		{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, DescriptorRef: "resource/r", Runtime: "r", Protocol: "mcp", Operations: map[string]knowledge.BindingOperation{"read": {Call: "x"}}}},
	}
	for i, source := range cases {
		if code := kernel.CodeOf(knowledge.ValidateValueSource(source)); code != kernel.ErrUsageInvalid {
			t.Fatalf("case %d must fail closed, got %s", i, code)
		}
	}
}

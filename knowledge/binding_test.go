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
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	source := func(call string) *knowledge.ValueSource {
		return &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
			Mode: knowledge.BindingState, Runtime: "orders", Protocol: "mcp",
			Operations: map[string]knowledge.BindingOperation{"read": {Call: call}},
		}}
	}
	c1, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: source("health.v1")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r1, err := s.Repo.ResolveAddress(address, c1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: source("health.v2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.Repo.ResolveAddress(address, c2)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Digest != r2.Digest || r1.DeclarationDigest == r2.DeclarationDigest {
		t.Fatalf("value digest must stay stable while declaration digest changes: r1=%#v r2=%#v", r1, r2)
	}
	history, err := s.Repo.Log(address.ObjectID, c2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 || history[0].DeclarationDigest == history[1].DeclarationDigest {
		t.Fatalf("LOG must retain declaration-only revisions: %#v", history)
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

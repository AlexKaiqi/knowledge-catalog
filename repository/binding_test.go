package repository_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

func TestBindingDeclarationChangeIsVersionedWhenValueIsUnchanged(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := kernel.Address{Kind: kernel.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	source := func(call string) *repository.ValueSource {
		return &repository.ValueSource{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{
			Mode: repository.BindingState, Runtime: "orders", Protocol: "mcp",
			Operations: map[string]repository.BindingOperation{"read": {Call: call}},
		}}
	}
	c1, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{{Op: repository.OpPut, Address: address, Value: nil, ValueSource: source("health.v1")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r1, err := s.Repo.ResolveAddress(address, c1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []repository.Operation{{Op: repository.OpPut, Address: address, Value: nil, ValueSource: source("health.v2")}},
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
	cases := []*repository.ValueSource{
		{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{Mode: repository.BindingState}},
		{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{Mode: "snapshot", Runtime: "r", Protocol: "mcp", Operations: map[string]repository.BindingOperation{"read": {Call: "x"}}}},
		{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{Mode: repository.BindingState, DescriptorRef: "resource/r", Runtime: "r", Protocol: "mcp", Operations: map[string]repository.BindingOperation{"read": {Call: "x"}}}},
	}
	for i, source := range cases {
		if code := kernel.CodeOf(repository.ValidateValueSource(source)); code != kernel.ErrUsageInvalid {
			t.Fatalf("case %d must fail closed, got %s", i, code)
		}
	}
}

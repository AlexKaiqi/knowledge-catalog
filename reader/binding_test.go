package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

func inlineBinding(mode repository.BindingMode, runtime string) *repository.ValueSource {
	return &repository.ValueSource{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{
		Mode: mode, Runtime: runtime, Protocol: "mcp",
		Operations: map[string]repository.BindingOperation{"read": {Call: "resource.read"}},
	}}
}

func TestResolveInlineStateAndStreamBindings(t *testing.T) {
	for _, mode := range []repository.BindingMode{repository.BindingState, repository.BindingStream} {
		t.Run(string(mode), func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			address := kernel.Address{Kind: kernel.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
			head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: []repository.Operation{{Op: repository.OpPut, Address: address, Value: nil, ValueSource: inlineBinding(mode, "orders-runtime")}},
			})
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Reader.ResolveBinding(s.RepositoryID, head, address)
			if err != nil || got.Mode != mode || got.Runtime != "orders-runtime" || got.Protocol != "mcp" || got.Operations["read"].Call != "resource.read" {
				t.Fatalf("%#v %v", got, err)
			}
			if got.DeclarationCommit != head || got.DeclarationDigest == "" {
				t.Fatalf("binding must carry its pinned declaration version: %#v", got)
			}
		})
	}
}

func TestResolveDescriptorBindingAtPinnedCommit(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := kernel.Address{Kind: kernel.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	descriptorID := kernel.ObjectID("resource/orders-runtime")
	descriptor := func(runtime string) map[string]any {
		return map[string]any{
			"kind": "ResourceDescriptor", "runtime": runtime, "protocol": "mcp",
			"access": map[string]any{"read": map[string]any{"call": "resource.read"}},
		}
	}
	c1, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: descriptorID}, Value: descriptor("runtime-v1")},
			{Op: repository.OpPut, Address: address, Value: nil, ValueSource: &repository.ValueSource{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{Mode: repository.BindingState, DescriptorRef: descriptorID}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []repository.Operation{{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: descriptorID}, Value: descriptor("runtime-v2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v1, err := reader.ResolveRepoBinding(s.Repo, c1, address)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := reader.ResolveRepoBinding(s.Repo, c2, address)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Runtime != "runtime-v1" || v2.Runtime != "runtime-v2" || v1.DescriptorDigest == v2.DescriptorDigest {
		t.Fatalf("descriptor must resolve at the declaration pin: v1=%#v v2=%#v", v1, v2)
	}
}

func TestResolveBindingRejectsMissingOrInvalidDescriptor(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := kernel.Address{Kind: kernel.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{{Op: repository.OpPut, Address: address, Value: nil, ValueSource: &repository.ValueSource{
			Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{Mode: repository.BindingState, DescriptorRef: "resource/missing"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.ResolveRepoBinding(s.Repo, head, address)
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)

	badDescriptor := kernel.ObjectID("resource/bad")
	head2, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: repository.DefaultRef,
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: []repository.Operation{
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: badDescriptor}, Value: map[string]any{
				"kind": "ResourceDescriptor", "runtime": "r", "protocol": "mcp", "access": map[string]any{"read": map[string]any{}},
			}},
			{Op: repository.OpPut, Address: address, Value: nil, ValueSource: &repository.ValueSource{Kind: repository.ValueSourceBinding, Binding: &repository.BindingDeclaration{Mode: repository.BindingState, DescriptorRef: badDescriptor}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.ResolveRepoBinding(s.Repo, head2, address)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

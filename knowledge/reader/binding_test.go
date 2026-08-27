package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func inlineBinding(mode knowledge.BindingMode, runtime string) *knowledge.ValueSource {
	return &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
		Mode: mode, Runtime: runtime, Protocol: "mcp",
		Operations: map[string]knowledge.BindingOperation{"read": {Call: "resource.read"}},
	}}
}

func TestResolveInlineStateAndStreamBindings(t *testing.T) {
	for _, mode := range []knowledge.BindingMode{knowledge.BindingState, knowledge.BindingStream} {
		t.Run(string(mode), func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
			head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: inlineBinding(mode, "orders-runtime")}},
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
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	descriptorID := knowledge.ObjectID("resource/orders-runtime")
	descriptor := func(runtime string) map[string]any {
		return map[string]any{
			"kind": "ResourceDescriptor", "runtime": runtime, "protocol": "mcp",
			"access": map[string]any{"read": map[string]any{"call": "resource.read"}},
		}
	}
	c1, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: descriptorID}, Value: descriptor("runtime-v1")},
			{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, DescriptorRef: descriptorID}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: c1, ExpectedTargetCommit: c1,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: descriptorID}, Value: descriptor("runtime-v2")}},
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

func TestResolveAspectResourceDescriptor(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:orders", AspectName: "profile"}
	descriptorID := knowledge.ObjectID("resource/mysql")
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: descriptorID, AspectName: "definition"}, Value: map[string]any{
				"kind": "ResourceDescriptor", "runtime": "mysql-adapter", "protocol": "mysql-adapter/v1",
				"access": map[string]any{"profile": map[string]any{"call": "mysql.profile"}},
			}},
			{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, DescriptorRef: descriptorID}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Reader.ResolveBinding(s.RepositoryID, head, address)
	if err != nil || got.Runtime != "mysql-adapter" || got.Operations["profile"].Call != "mysql.profile" {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestResolveBindingRejectsMissingOrInvalidDescriptor(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: &knowledge.ValueSource{
			Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, DescriptorRef: "resource/missing"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.ResolveRepoBinding(s.Repo, head, address)
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)

	badDescriptor := knowledge.ObjectID("resource/bad")
	head2, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: badDescriptor}, Value: map[string]any{
				"kind": "ResourceDescriptor", "runtime": "r", "protocol": "mcp", "access": map[string]any{"read": map[string]any{}},
			}},
			{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, DescriptorRef: badDescriptor}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.ResolveRepoBinding(s.Repo, head2, address)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

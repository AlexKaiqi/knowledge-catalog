package reader_test

import (
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"testing"
)

// Legacy declaration fixture: new Writer rejects instance Bindings, while the
// reader must still honor fixed-basis descriptors already stored by a source.
type descriptorPinRepository struct{ knowledge.Repository }

func (r descriptorPinRepository) SchemaObjectIDs(kernel.CommitID) ([]knowledge.ObjectID, error) {
	return nil, nil
}
func (r descriptorPinRepository) Read(id knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	value := any(map[string]any{"name": "orders"})
	if id == "resource/health" {
		value = map[string]any{"kind": "ResourceDescriptor", "runtime": string(commit), "protocol": "mcp", "access": map[string]any{"read": map[string]any{"call": "resource.read"}}}
	}
	return knowledge.KnowledgeValue{Repository: r.ID(), Commit: commit, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: id}, Value: value}, nil
}
func (r descriptorPinRepository) ResolveAddress(addr knowledge.Address, commit kernel.CommitID) (knowledge.Resolution, error) {
	return knowledge.Resolution{Status: knowledge.StatusResolved, Address: addr, Commit: commit, ValueSource: &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, DescriptorRef: "resource/health"}}}, nil
}
func TestResolveDescriptorBindingAtPinnedCommit(t *testing.T) {
	s := testkit.NewSetup(t, "")
	repo := descriptorPinRepository{Repository: s.Repo}
	addr := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	old, err := reader.ResolveRepoBinding(repo, "old-runtime", addr)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := reader.ResolveRepoBinding(repo, "new-runtime", addr)
	if err != nil {
		t.Fatal(err)
	}
	again, err := reader.ResolveRepoBinding(repo, "old-runtime", addr)
	if err != nil {
		t.Fatal(err)
	}
	if old.Runtime != "old-runtime" || newer.Runtime != "new-runtime" || old.DescriptorDigest == newer.DescriptorDigest || again.DescriptorDigest != old.DescriptorDigest || again.DeclarationCommit != "old-runtime" {
		t.Fatalf("descriptor drift: %#v %#v %#v", old, newer, again)
	}
}

package liveaccess_test

import (
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"testing"
)

// Legacy declaration fixture: new Writer rejects instance Bindings, while the
// reader must still honor fixed-basis descriptors already stored by a source.
type descriptorPinRepository struct{ objectmodel.Repository }

func (r descriptorPinRepository) SchemaObjectIDs(kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return nil, nil
}
func (r descriptorPinRepository) Read(id objectmodel.ObjectID, commit kernel.CommitID) (objectmodel.ObjectValue, error) {
	value := any(map[string]any{"name": "orders"})
	if id == "resource/health" {
		value = map[string]any{"kind": "ResourceDescriptor", "runtime": string(commit), "protocol": "mcp", "access": map[string]any{"read": map[string]any{"call": "resource.read"}}}
	}
	return objectmodel.ObjectValue{Repository: r.ID(), Commit: commit, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, Value: value}, nil
}
func (r descriptorPinRepository) ResolveAddress(addr objectmodel.Address, commit kernel.CommitID) (objectmodel.Resolution, error) {
	return objectmodel.Resolution{Status: objectmodel.StatusResolved, Address: addr, Commit: commit, ValueSource: &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{Mode: objectmodel.BindingState, DescriptorRef: "resource/health"}}}, nil
}
func TestResolveDescriptorBindingAtPinnedCommit(t *testing.T) {
	s := testkit.NewSetup(t, "")
	repo := descriptorPinRepository{Repository: s.Repo}
	addr := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	old, err := liveaccess.ResolveRepoBinding(repo, "old-runtime", addr)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := liveaccess.ResolveRepoBinding(repo, "new-runtime", addr)
	if err != nil {
		t.Fatal(err)
	}
	again, err := liveaccess.ResolveRepoBinding(repo, "old-runtime", addr)
	if err != nil {
		t.Fatal(err)
	}
	if old.Runtime != "old-runtime" || newer.Runtime != "new-runtime" || old.DescriptorDigest == newer.DescriptorDigest || again.DescriptorDigest != old.DescriptorDigest || again.DeclarationCommit != "old-runtime" {
		t.Fatalf("descriptor drift: %#v %#v %#v", old, newer, again)
	}
}

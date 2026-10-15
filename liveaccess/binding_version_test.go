package liveaccess_test

import (
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

// The declaration version covers the unit and the adopted descriptor: changing
// either one versions the Binding while the entity Snapshot stays unchanged.
func TestBindingDeclarationChangeIsVersionedWhenValueIsUnchanged(t *testing.T) {
	s := testkit.NewSetup(t, "")
	descriptor := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "resource/health"}
	entity := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "properties"}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	commit := func(base kernel.CommitID, ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		next, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
			TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
		})
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	resolve := func(at kernel.CommitID) liveaccess.ResolvedBinding {
		t.Helper()
		got, err := liveaccess.ResolveRepoBinding(s.Repo, at, address, liveaccess.RepositoryDescriptors(s.Repo, at))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	c1 := commit(s.RootCommitID,
		bindingfixture.PutDescriptor(descriptor.ObjectID, bindingfixture.Descriptor("https://stats.example/v1")),
		bindingfixture.PutDescriptor("resource/health-next", bindingfixture.Descriptor("https://next.example")),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: entity, Value: map[string]any{"name": "orders"}},
		bindingfixture.PutAspect(address.ObjectID, address.AspectName, "", descriptor.ObjectID),
	)
	b1 := resolve(c1)
	c2 := commit(c1, bindingfixture.PutDescriptor(descriptor.ObjectID, bindingfixture.Descriptor("https://stats.example/v2")))
	b2 := resolve(c2)
	c3 := commit(c2, bindingfixture.PutAspect(address.ObjectID, address.AspectName, "", "resource/health-next"))
	b3 := resolve(c3)
	r1, err := s.Repo.ResolveAddress(entity, c1)
	if err != nil {
		t.Fatal(err)
	}
	r3, err := s.Repo.ResolveAddress(entity, c3)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Digest != r3.Digest {
		t.Fatalf("entity Snapshot digest must stay stable: r1=%#v r3=%#v", r1, r3)
	}
	if b1.DeclarationDigest == b2.DeclarationDigest || b1.Origin == b2.Origin || b1.DescriptorDigest == b2.DescriptorDigest {
		t.Fatalf("descriptor change must version the Binding declaration: b1=%#v b2=%#v", b1, b2)
	}
	if b2.DeclarationDigest == b3.DeclarationDigest || b3.DescriptorRef != "resource/health-next" {
		t.Fatalf("unit descriptorRef change must version the Binding declaration: b2=%#v b3=%#v", b2, b3)
	}
	history, err := s.Repo.Log(descriptor.ObjectID, c3, objectmodel.ObjectLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 {
		t.Fatalf("LOG must retain ResourceDescriptor revisions: %#v", history)
	}
}

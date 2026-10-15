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

func publishHealthBinding(t *testing.T, s testkit.Setup, origin string) kernel.CommitID {
	t.Helper()
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
			bindingfixture.PutDescriptor("resource/health", bindingfixture.Descriptor(origin)),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
			bindingfixture.PutAspect("service/orders", "health", "schema/service.health", "resource/health"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestResolveBindingReadsDescriptorOrigin(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head := publishHealthBinding(t, s, "https://stats.example")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	got, err := liveaccess.ResolveRepoBinding(s.Repo, head, address, liveaccess.RepositoryDescriptors(s.Repo, head))
	if err != nil || got.Origin != "https://stats.example" || got.SchemaRef != "schema/service.health" || got.Mode != objectmodel.BindingState ||
		got.Operations["lookup"].Call != "lookup" || got.DescriptorRef != "resource/health" || got.DescriptorRepository != s.RepositoryID || got.DescriptorCommit != head {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestBindingUnitOccupiesNoSnapshotValue(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head := publishHealthBinding(t, s, "https://stats.example")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	resolution, err := s.Repo.ResolveAddress(address, head)
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.ValueSource.IsBinding() {
		t.Fatalf("binding unit lost its declaration: %#v", resolution)
	}
	value, err := s.Repo.ReadAddress(address, head)
	if err != nil {
		t.Fatal(err)
	}
	if value.Value != nil {
		t.Fatalf("binding unit must not carry a Snapshot value: %#v", value.Value)
	}
}

func TestResolveBindingRequiresDescriptorInScope(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head := publishHealthBinding(t, s, "https://stats.example")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	if _, err := liveaccess.ResolveRepoBinding(s.Repo, head, address, nil); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("no request scope must be CAPABILITY_UNSATISFIED: %v", err)
	}
	empty := func(id objectmodel.ObjectID) (liveaccess.DescriptorAt, error) {
		return liveaccess.DescriptorAt{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "%s outside scope", id)
	}
	if _, err := liveaccess.ResolveRepoBinding(s.Repo, head, address, empty); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("descriptor outside the request scope must be OBJECT_REF_UNRESOLVED: %v", err)
	}
	notDescriptor := func(objectmodel.ObjectID) (liveaccess.DescriptorAt, error) {
		return liveaccess.DescriptorAt{Repository: s.RepositoryID, Commit: head, Value: map[string]any{"name": "plain"}}, nil
	}
	if _, err := liveaccess.ResolveRepoBinding(s.Repo, head, address, notDescriptor); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("a non-descriptor target must be USAGE_INVALID: %v", err)
	}
}

func TestApplyObjectCommitRejectsInlineBindingAccess(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	_, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: address, ValueSource: &objectmodel.ValueSource{
			Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{},
		}}},
	})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("binding without descriptorRef must be rejected: %v", err)
	}
}

func TestResolveBindingWithoutBindingUnitIsUnsatisfied(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
			{Op: objectmodel.OpPut, Address: address, Value: map[string]any{"status": "ok"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = liveaccess.ResolveRepoBinding(s.Repo, head, address, liveaccess.RepositoryDescriptors(s.Repo, head))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("a Snapshot unit has no binding declaration: %v", err)
	}
}

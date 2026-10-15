package liveaccess_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestResolveBindingReadsSchemaOrigin(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health", "origin": "https://stats.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := liveaccess.ResolveRepoBinding(s.Repo, head, address)
	if err != nil || got.Origin != "https://stats.example" || got.SchemaRef != "schema/service.health" {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestResolveBindingFromSchemaWithoutInstanceFile(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.stats"}, Value: map[string]any{
				"entity": "Table", "aspect": "stats", "origin": "https://stats.example",
				"fields": map[string]any{"rowCount": map[string]any{"type": "number"}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := liveaccess.ResolveRepoBinding(s.Repo, head, address)
	if err != nil || got.Origin != "https://stats.example" || got.Mode != objectmodel.BindingState || got.Operations["lookup"].Call != "lookup" {
		t.Fatalf("%#v %v", got, err)
	}
	resolution, err := s.Repo.ResolveAddress(address, head)
	if err != nil {
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			t.Fatal(err)
		}
		return
	}
	if resolution.Status == objectmodel.StatusResolved {
		t.Fatalf("stats must not occupy a Snapshot unit: %#v", resolution)
	}
}

func TestApplyObjectCommitRejectsInstanceBinding(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	_, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: address, Value: nil, ValueSource: &objectmodel.ValueSource{
			Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{
				Mode: objectmodel.BindingState, Runtime: "orders-runtime", Protocol: "mcp",
				Operations: map[string]objectmodel.BindingOperation{"read": {Call: "resource.read"}},
			},
		}}},
	})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("instance Binding must be rejected: %v", err)
	}
}

func TestResolveBindingWithoutBoundSchemaIsUnsatisfied(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "properties"}, Value: map[string]any{"name": "orders"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = liveaccess.ResolveRepoBinding(s.Repo, head, address)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing Bound Schema must be CAPABILITY_UNSATISFIED: %v", err)
	}
}

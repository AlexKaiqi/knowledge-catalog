package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestResolveBindingReadsSchemaOrigin(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health", "origin": "https://stats.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Reader.ResolveBinding(s.RepositoryID, head, address)
	if err != nil || got.Origin != "https://stats.example" || got.SchemaRef != "schema/service.health" {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestResolveBindingFromSchemaWithoutInstanceFile(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "table/orders", AspectName: "stats"}
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/table.stats"}, Value: map[string]any{
				"entity": "Table", "aspect": "stats", "origin": "https://stats.example",
				"fields": map[string]any{"rowCount": map[string]any{"type": "number"}},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "table/orders", AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.ResolveRepoBinding(s.Repo, head, address)
	if err != nil || got.Origin != "https://stats.example" || got.Mode != knowledge.BindingState || got.Operations["lookup"].Call != "lookup" {
		t.Fatalf("%#v %v", got, err)
	}
	resolution, err := s.Repo.ResolveAddress(address, head)
	if err != nil {
		if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
			t.Fatal(err)
		}
		return
	}
	if resolution.Status == knowledge.StatusResolved {
		t.Fatalf("stats must not occupy a Snapshot unit: %#v", resolution)
	}
}

func TestApplyKnowledgeCommitRejectsInstanceBinding(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	_, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: &knowledge.ValueSource{
			Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
				Mode: knowledge.BindingState, Runtime: "orders-runtime", Protocol: "mcp",
				Operations: map[string]knowledge.BindingOperation{"read": {Call: "resource.read"}},
			},
		}}},
	})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("instance Binding must be rejected: %v", err)
	}
}

func TestResolveBindingWithoutBoundSchemaIsUnsatisfied(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: address.ObjectID, AspectName: "properties"}, Value: map[string]any{"name": "orders"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.ResolveRepoBinding(s.Repo, head, address)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing Bound Schema must be CAPABILITY_UNSATISFIED: %v", err)
	}
}

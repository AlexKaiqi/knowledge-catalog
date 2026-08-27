package cli

import (
	"context"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/observability"
	"kc/retrieval"
	"kc/snapshot"
)

type searchStateLookup struct{}

func (searchStateLookup) LookupState(_ context.Context, _ knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	return knowledgeserving.StateObservation{
		Value: map[string]any{"status": "healthy"},
		Basis: knowledge.ObservationBasis{
			BindingGeneration: "runtime-v1", Consistency: knowledge.ObservationRepeatable,
			SourceRevision: "health-7", ObservedAt: "2026-08-27T13:00:00Z",
		},
	}, nil
}

func TestWorkspaceSearchHitUsesLogicalStateHydration(t *testing.T) {
	s := testkit.NewSetup(t, "")
	objectID := knowledge.ObjectID("Service:orders")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: objectID, AspectName: "health"}
	commit, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: objectID, AspectName: "definition"}, Value: map[string]any{"name": "Orders"}},
			{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: &knowledge.ValueSource{
				Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
					Mode: knowledge.BindingState, Runtime: "health", Protocol: "resource-access/v1",
					Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "health.lookup"}},
				},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.Repo.Read(objectID, commit)
	if err != nil {
		t.Fatal(err)
	}
	declarations := reader.Open(func(repositoryID kernel.RepositoryID) (knowledge.Repository, error) {
		return s.Repo, nil
	}, reader.WorkspacePin{WorkspaceID: "agent", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}})
	logical := knowledgeserving.Open(declarations, searchStateLookup{}, observability.IdentityContext{Principal: "agent"})
	hit, err := hydrateSearchHit(context.Background(), logical, retrieval.KnowledgeHit{
		Knowledge: raw, Version: retrieval.VersionOf(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	value := hit.Knowledge.Value.(map[string]any)
	if value["health"].(map[string]any)["status"] != "healthy" {
		t.Fatalf("search returned raw Binding placeholder: %#v", value)
	}
	if len(hit.Version.Observations) != 1 || hit.Version.Observations[0].Basis.SourceRevision != "health-7" {
		t.Fatalf("search lost observation basis: %#v", hit.Version)
	}
}

package invocation

import (
	"context"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	knowledgeserving "kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/observability"
	"kc/snapshot"
	"testing"
)

type searchStateLookup struct{}

func (searchStateLookup) LookupState(_ context.Context, _ knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	return knowledgeserving.StateObservation{
		Value: map[string]any{"status": "healthy"},
		Basis: objectmodel.ObservationBasis{
			BindingGeneration: "runtime-v1", Consistency: objectmodel.ObservationRepeatable,
			SourceRevision: "health-7", ObservedAt: "2026-08-27T13:00:00Z",
		},
	}, nil
}

func TestWorkspaceSearchHitUsesLogicalStateHydration(t *testing.T) {
	s := testkit.NewSetup(t, "")
	objectID := objectmodel.ObjectID("Service:orders")
	commit, err := s.Repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectID, AspectName: "definition"}, Value: map[string]any{"name": "Orders"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health", "origin": "https://health.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"text", "filter"}}},
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
	declarations := reader.Open(func(repositoryID kernel.RepositoryID) (objectmodel.Repository, error) {
		return s.Reader.Require(repositoryID, kernel.ErrKnowledgeRefUnresolved)
	}, reader.KnowledgeSetPin{SetID: "agent", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit})})
	logical := liveservice.Open(declarations, searchStateLookup{}, observability.IdentityContext{Principal: "agent"})
	hit, err := HydrateSearchHit(context.Background(), logical, retrieval.KnowledgeHit{
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

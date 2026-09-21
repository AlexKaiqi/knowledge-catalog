package knowledgeapp

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

type scopedStatePlanner struct {
	searchProjection
	t *testing.T
}

func (p *scopedStatePlanner) RequiresState(repo knowledge.Repository, at kernel.CommitID, _ retrieval.SearchRequest) (bool, error) {
	if _, err := repo.Read("schema/private", at); err == nil {
		p.t.Fatal("State planning received an unscoped repository")
	}
	return false, nil
}

func TestDatasetStatePlanningUsesPublishedSchemaScope(t *testing.T) {
	s := testkit.NewSetup(t, "")
	at, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/public"}, PathHint: "public/schema.yaml", Value: map[string]any{"entity": "Public", "fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/private"}, PathHint: "private/schema.yaml", Value: map[string]any{"entity": "Private", "aspect": "runtime", "origin": "https://scheduler.example", "fields": map[string]any{"secret": map[string]any{"type": "string", "access": []any{"filter"}}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	scope := reader.Open(func(kernel.RepositoryID) (knowledge.Repository, error) { return s.Repo, nil }, reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: at}, Items: []reader.DatasetItem{{Repository: s.RepositoryID, Commit: at, Kind: "prefix", Prefix: "public"}}})
	e := DatasetSearchExecutor{Authorize: func(context.Context) error { return nil }, Repositories: searchLookup{repo: s.Repo}, Projection: &scopedStatePlanner{t: t},
		Resolve: func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
			return scope, knowledgeserving.Open(scope, nil, observability.IdentityContext{}), nil
		},
		Deliver: func(_ context.Context, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) { return hit, nil },
	}
	if _, err := e.Execute(context.Background(), retrieval.SearchOf(retrieval.SearchEQ("status", "yes"))); err != nil {
		t.Fatal(err)
	}
}

func TestDatasetStateObservationRequiresPublishedDeclaration(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Job:one"}, PathHint: "public/job.yaml", Value: map[string]any{"name": "one"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/runtime"}, PathHint: "private/runtime.yaml", Value: map[string]any{"entity": "Job", "aspect": "runtime", "origin": "https://scheduler.example", "fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Job:one", AspectName: "runtime"}
	binding, err := reader.ResolveRepoBinding(s.Repo, commit, address)
	if err != nil {
		t.Fatal(err)
	}
	hit := retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{Repository: s.RepositoryID, KnowledgeRef: knowledge.KnowledgeRef{Repository: s.RepositoryID, Object: address.ObjectID}}, Version: retrieval.KnowledgeVersion{Observations: []knowledge.UnitObservation{{Address: address, DeclarationCommit: commit, DeclarationDigest: binding.DeclarationDigest}}}}
	lookup := func(kernel.RepositoryID) (knowledge.Repository, error) { return s.Repo, nil }
	pin := reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}, Items: []reader.DatasetItem{{Repository: s.RepositoryID, Commit: commit, Kind: "prefix", Prefix: "public"}}}
	if err := validateDatasetObservations(reader.Open(lookup, pin), hit); err == nil {
		t.Fatal("State cache exposed an unpublished declaration")
	}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	full := reader.Open(lookup, pin)
	if err := validateDatasetObservations(full, hit); err != nil {
		t.Fatal(err)
	}
	hit.Version.Observations[0].DeclarationCommit = s.RootCommitID
	if err := validateDatasetObservations(full, hit); err == nil {
		t.Fatal("State cache from a different declaration version was accepted")
	}
}

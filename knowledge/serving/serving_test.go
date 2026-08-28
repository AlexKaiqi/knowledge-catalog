package serving_test

import (
	"context"
	"errors"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/observability"
	"kc/snapshot"
)

type stateLookup struct {
	requests []serving.StateLookupRequest
	result   serving.StateObservation
	err      error
}

func (s *stateLookup) LookupState(_ context.Context, request serving.StateLookupRequest) (serving.StateObservation, error) {
	s.requests = append(s.requests, request)
	return s.result, s.err
}

func stateBinding(mode knowledge.BindingMode) *knowledge.ValueSource {
	return &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
		Mode: mode, Runtime: "scheduler", Protocol: "scheduler/v1",
		Operations: map[string]knowledge.BindingOperation{"read": {Call: "job.status"}},
	}}
}

func setupServing(t *testing.T, mode knowledge.BindingMode) (*reader.Serving, kernel.RepositoryID, kernel.CommitID, knowledge.Address) {
	t.Helper()
	s := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Job:orders", AspectName: "runtime"}
	commit, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "data"}},
			{Op: knowledge.OpPut, Address: address, Value: nil, ValueSource: stateBinding(mode)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := reader.Open(func(id kernel.RepositoryID) (knowledge.Repository, error) {
		if id != s.RepositoryID {
			return nil, errors.New("unexpected repository")
		}
		return s.Repo, nil
	}, reader.WorkspacePin{WorkspaceID: "agent", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}})
	return base, s.RepositoryID, commit, address
}

func TestStateBindingHydratesConsumerReadAndKeepsBothBases(t *testing.T) {
	base, repositoryID, commit, address := setupServing(t, knowledge.BindingState)
	lookup := &stateLookup{result: serving.StateObservation{
		Value: map[string]any{"status": "running", "progress": float64(70)},
		Basis: knowledge.ObservationBasis{
			BindingGeneration: "scheduler-config-7", Consistency: knowledge.ObservationRepeatable,
			SourceRevision: "job-rev-42", ObservedAt: "2026-08-27T08:30:00Z",
		},
	}}
	identity := observability.IdentityContext{Principal: "agent", OnBehalfOf: "alice"}
	service := serving.Open(base, lookup, identity)

	results, err := service.Read(context.Background(), address.ObjectID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("%#v", results)
	}
	value := results[0].Value.(map[string]any)
	runtime := value["runtime"].(map[string]any)
	if runtime["status"] != "running" || runtime["progress"] != float64(70) {
		t.Fatalf("bound value was not hydrated: %#v", value)
	}
	if value["definition"].(map[string]any)["owner"] != "data" {
		t.Fatalf("snapshot aspect was not preserved: %#v", value)
	}
	if results[0].Commit != commit || len(results[0].Observations) != 1 {
		t.Fatalf("result lost declaration/observation versions: %#v", results[0])
	}
	version := results[0].Observations[0]
	if version.Address != address || version.DeclarationCommit != commit || version.DeclarationDigest == "" || version.Basis.SourceRevision != "job-rev-42" {
		t.Fatalf("observation version: %#v", version)
	}
	if len(lookup.requests) != 1 || lookup.requests[0].Binding.Repository != repositoryID || lookup.requests[0].Identity != identity {
		t.Fatalf("lookup request did not preserve binding and identity: %#v", lookup.requests)
	}

	raw, err := base.ReadAddress(address)
	if err != nil || len(raw) != 1 || raw[0].Value != nil {
		t.Fatalf("declaration Reader must remain raw: %#v %v", raw, err)
	}
}

func TestStateBindingSelectionAvoidsUnrequestedLookup(t *testing.T) {
	base, _, _, address := setupServing(t, knowledge.BindingState)
	lookup := &stateLookup{result: serving.StateObservation{
		Value: map[string]any{"status": "running"},
		Basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationLatestOnly, ObservedAt: "2026-08-27T08:30:00Z"},
	}}
	service := serving.Open(base, lookup, observability.IdentityContext{Principal: "agent"})
	results, err := service.Read(context.Background(), address.ObjectID, &knowledge.AspectSelector{Include: []string{"definition"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.requests) != 0 {
		t.Fatalf("excluded Binding caused a runtime call: %#v", lookup.requests)
	}
	value := results[0].Value.(map[string]any)
	if len(value) != 1 || value["definition"] == nil {
		t.Fatalf("selector result: %#v", value)
	}
}

func TestBoundReadFailsClosedWithoutStateRuntime(t *testing.T) {
	base, _, _, address := setupServing(t, knowledge.BindingState)
	_, err := serving.Open(base, nil, observability.IdentityContext{Principal: "agent"}).ReadAddress(context.Background(), address)
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
}

func TestOrdinaryReadRejectsStreamBinding(t *testing.T) {
	base, _, _, address := setupServing(t, knowledge.BindingStream)
	lookup := &stateLookup{}
	_, err := serving.Open(base, lookup, observability.IdentityContext{Principal: "agent"}).Read(context.Background(), address.ObjectID, nil)
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
	if len(lookup.requests) != 0 {
		t.Fatal("ordinary READ must not call a State runtime for a Stream Binding")
	}
}

func TestStateRuntimeFailuresAndInvalidBasisFailHonestly(t *testing.T) {
	base, _, _, address := setupServing(t, knowledge.BindingState)
	lookup := &stateLookup{err: errors.New("connection reset")}
	_, err := serving.Open(base, lookup, observability.IdentityContext{Principal: "agent"}).ReadAddress(context.Background(), address)
	testkit.ExpectCode(t, err, kernel.ErrTemporaryUnavailable)

	lookup.err = nil
	lookup.result = serving.StateObservation{Value: true, Basis: knowledge.ObservationBasis{BindingGeneration: "g1", Consistency: knowledge.ObservationLatestOnly}}
	_, err = serving.Open(base, lookup, observability.IdentityContext{Principal: "agent"}).ReadAddress(context.Background(), address)
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
}

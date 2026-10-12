package serving_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/snapshot"
)

func TestLiveAccessUsesPinnedTargetAndSeparatesObservation(t *testing.T) {
	var received serving.ResourceOperationRequest
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/access" || r.Header.Get("X-Resource-Principal") != "agent" {
			t.Errorf("runtime target or identity changed: %s %#v", r.URL, r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": nil, "basis": knowledge.ObservationBasis{BindingGeneration: "source-v2", Consistency: knowledge.ObservationBounded, SourceRevision: "rev-8", ObservedAt: "2026-10-10T03:00:00Z"}})
	}))
	defer runtime.Close()
	impl := reader.ResolvedInterface{Repository: "kr://test/live", Commit: "pinned-7", Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "table/orders", AspectName: "interfaces", MemberKey: "current"}, Implementation: map[string]any{"runtime": "warehouse", "protocol": knowledge.ProtocolResourceAccessV1, "origin": runtime.URL, "operations": map[string]any{"read": map[string]any{"call": "orders.current"}}}}
	handler := serving.NewLiveAccessHandler(serving.NewHTTPStateLookup(nil))
	if err := handler.ValidateImplementation(context.Background(), impl); err != nil {
		t.Fatal(err)
	}
	result, err := handler.Execute(context.Background(), serving.InterfaceCall{Implementation: impl, Operation: "read", Input: map[string]any{"origin": "https://untrusted.invalid", "operation": "restart", "principal": "admin"}, Request: serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}, RequestID: "request-9"}})
	if err != nil {
		t.Fatal(err)
	}
	basis, ok := result.Basis.(knowledge.ObservationBasis)
	if result.Value != nil || !ok || basis.SourceRevision != "rev-8" || received.Descriptor.Commit != "pinned-7" || received.Call != "orders.current" || received.Operation != "read" || received.Identity.Principal != "agent" {
		t.Fatalf("observation or pinned execution changed: result=%#v request=%#v", result, received)
	}
}

func TestLiveAccessRejectsMissingObservationAndUndeclaredOperation(t *testing.T) {
	accessor := &liveAccessor{value: map[string]any{"value": "healthy"}}
	handler := serving.NewLiveAccessHandler(accessor)
	impl := reader.ResolvedInterface{Implementation: map[string]any{"runtime": "runtime", "protocol": knowledge.ProtocolResourceAccessV1, "origin": "https://source.example", "operations": map[string]any{"read": map[string]any{"call": "read"}}}}
	call := serving.InterfaceCall{Implementation: impl, Operation: "read", Request: serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}}}
	if _, err := handler.Execute(context.Background(), call); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing observation basis: %v", err)
	}
	call.Operation = "invoke"
	if _, err := handler.Execute(context.Background(), call); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || accessor.calls != 1 {
		t.Fatalf("undeclared operation reached runtime: %v calls=%d", err, accessor.calls)
	}
}

type liveAccessor struct {
	value any
	calls int
}

func (a *liveAccessor) AccessResource(context.Context, serving.ResourceOperationRequest) (any, error) {
	a.calls++
	return a.value, nil
}

func TestLiveAccessRegistrationUsesPublicationAndWriteAuthorization(t *testing.T) {
	s := testkit.NewSetup(t, "")
	operations := knowledge.SystemSchemaOperations()
	for _, operation := range knowledge.SystemPublicationOperations() {
		if operation.Address.ObjectID == knowledge.LiveAccessInterfaceV1 {
			operations = append(operations, operation)
		}
	}
	operations = append(operations, knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "table/orders", AspectName: "interfaces", MemberKey: "live"}, SchemaRef: string(knowledge.LiveAccessImplementationSchemaV1), Value: map[string]any{"interfaceRef": string(knowledge.LiveAccessInterfaceV1), "runtime": "source", "protocol": knowledge.ProtocolResourceAccessV1, "origin": "https://source.example", "operations": map[string]any{"read": map[string]any{"call": "state"}, "invoke": map[string]any{"call": "preview"}}}})
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: head}
	declarations := reader.Open(func(kernel.RepositoryID) (knowledge.Repository, error) { return s.Repo, nil }, reader.KnowledgeSetPin{SetID: "live", Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
	implementation, err := declarations.ResolveObjectInterfaceAt(s.RepositoryID, "table/orders", "live")
	if err != nil {
		t.Fatal(err)
	}
	accessor := &liveAccessor{value: map[string]any{"status": "accepted"}}
	registry := serving.NewInterfaceRegistry()
	if err := serving.RegisterLiveAccess(registry, implementation, accessor); err != nil {
		t.Fatal(err)
	}
	changed := implementation
	changed.ContractDigest = "edited-contract"
	if err := serving.RegisterLiveAccess(serving.NewInterfaceRegistry(), changed, accessor); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("edited contract accepted: %v", err)
	}
	service := serving.OpenInterfaces(declarations, registry, serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	request := serving.InterfaceRequest{Repository: s.RepositoryID, Object: "table/orders", Implementation: "live", Operation: "invoke", Input: map[string]any{}}
	if _, err := service.Call(context.Background(), request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || accessor.calls != 0 {
		t.Fatalf("write reached runtime without authorization: %v calls=%d", err, accessor.calls)
	}
	service.SetAuthorizer(func(context.Context, serving.InterfaceCall) error { return nil })
	result, err := service.Call(context.Background(), request)
	if err != nil || result.Value.(map[string]any)["status"] != "accepted" || accessor.calls != 1 {
		t.Fatalf("authorized invocation: %#v %v calls=%d", result, err, accessor.calls)
	}
}

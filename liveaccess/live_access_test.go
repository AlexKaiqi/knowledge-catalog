package liveaccess_test

import (
	"context"
	"encoding/json"
	liveaccesscontract "kc/liveaccess/contract"
	"kc/liveaccess/dispatch"
	interfaces "kc/liveaccess/handler"
	"kc/systemrepo"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	serving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
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
		_ = json.NewEncoder(w).Encode(map[string]any{"value": nil, "basis": objectmodel.ObservationBasis{BindingGeneration: "source-v2", Consistency: objectmodel.ObservationBounded, SourceRevision: "rev-8", ObservedAt: "2026-10-10T03:00:00Z"}})
	}))
	defer runtime.Close()
	impl := interfaces.ResolvedInterface{Repository: "kr://test/live", Commit: "pinned-7", Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "interfaces", MemberKey: "current"}, Implementation: map[string]any{"runtime": "warehouse", "protocol": objectmodel.ProtocolResourceAccessV1, "origin": runtime.URL, "operations": map[string]any{"read": map[string]any{"call": "orders.current"}}}}
	handler := serving.NewLiveAccessHandler(serving.NewHTTPStateLookup(nil))
	if err := handler.ValidateImplementation(context.Background(), impl); err != nil {
		t.Fatal(err)
	}
	result, err := handler.Execute(context.Background(), interfaces.InterfaceCall{Implementation: impl, Operation: "read", Input: map[string]any{"origin": "https://untrusted.invalid", "operation": "restart", "principal": "admin"}, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}, RequestID: "request-9"}})
	if err != nil {
		t.Fatal(err)
	}
	basis, ok := result.Basis.(objectmodel.ObservationBasis)
	if result.Value != nil || !ok || basis.SourceRevision != "rev-8" || received.Descriptor.Commit != "pinned-7" || received.Call != "orders.current" || received.Operation != "read" || received.Identity.Principal != "agent" {
		t.Fatalf("observation or pinned execution changed: result=%#v request=%#v", result, received)
	}
}

func TestLiveAccessRejectsMissingObservationAndUndeclaredOperation(t *testing.T) {
	accessor := &liveAccessor{value: map[string]any{"value": "healthy"}}
	handler := serving.NewLiveAccessHandler(accessor)
	impl := interfaces.ResolvedInterface{Implementation: map[string]any{"runtime": "runtime", "protocol": objectmodel.ProtocolResourceAccessV1, "origin": "https://source.example", "operations": map[string]any{"read": map[string]any{"call": "read"}}}}
	call := interfaces.InterfaceCall{Implementation: impl, Operation: "read", Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}}}
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
	operations := systemrepo.SystemSchemaOperations()
	for _, operation := range systemrepo.SystemPublicationOperations() {
		if operation.Address.ObjectID == liveaccesscontract.LiveAccessInterfaceV1 {
			operations = append(operations, operation)
		}
	}
	operations = append(operations, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "table/orders", AspectName: "interfaces", MemberKey: "live"}, SchemaRef: string(liveaccesscontract.LiveAccessImplementationSchemaV1), Value: map[string]any{"interfaceRef": string(liveaccesscontract.LiveAccessInterfaceV1), "runtime": "source", "protocol": objectmodel.ProtocolResourceAccessV1, "origin": "https://source.example", "operations": map[string]any{"read": map[string]any{"call": "state"}, "invoke": map[string]any{"call": "preview"}}}})
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: head}
	declarations := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo, nil }, reader.DatasetPin{SetID: "live", Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
	implementation, err := interfaces.ResolveObjectInterfaceAt(declarations, s.RepositoryID, "table/orders", "live")
	if err != nil {
		t.Fatal(err)
	}
	accessor := &liveAccessor{value: map[string]any{"status": "accepted"}}
	registry := interfaces.NewInterfaceRegistry()
	if err := serving.RegisterLiveAccess(registry, implementation, accessor); err != nil {
		t.Fatal(err)
	}
	changed := implementation
	changed.ContractDigest = "edited-contract"
	if err := serving.RegisterLiveAccess(interfaces.NewInterfaceRegistry(), changed, accessor); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("edited contract accepted: %v", err)
	}
	service := dispatch.Open(declarations, registry, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	request := interfaces.InterfaceRequest{Repository: s.RepositoryID, Object: "table/orders", Implementation: "live", Operation: "invoke", Input: map[string]any{}}
	if _, err := service.Call(context.Background(), request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || accessor.calls != 0 {
		t.Fatalf("write reached runtime without authorization: %v calls=%d", err, accessor.calls)
	}
	service.SetAuthorizer(func(context.Context, interfaces.InterfaceCall) error { return nil })
	result, err := service.Call(context.Background(), request)
	if err != nil || result.Value.(map[string]any)["status"] != "accepted" || accessor.calls != 1 {
		t.Fatalf("authorized invocation: %#v %v calls=%d", result, err, accessor.calls)
	}
}

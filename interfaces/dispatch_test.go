package interfaces_test

import (
	"context"
	"kc/interfaces"
	interfaceservice "kc/interfaces/service"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"

	"kc/snapshot"
)

type prefixInterfaceHandler struct {
	calls   int
	invalid bool
}

func (h *prefixInterfaceHandler) ValidateImplementation(_ context.Context, impl interfaces.ResolvedInterface) error {
	if _, ok := impl.Implementation["prefix"].(string); !ok {
		return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "missing prefix")
	}
	return nil
}
func (h *prefixInterfaceHandler) Execute(_ context.Context, call interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
	h.calls++
	if h.invalid {
		return interfaces.InterfaceResult{Value: map[string]any{"text": 42}}, nil
	}
	return interfaces.InterfaceResult{Value: map[string]any{"text": call.Implementation.Implementation["prefix"].(string) + call.Input.(map[string]any)["text"].(string)}}, nil
}

func interfaceFixture(t *testing.T) (*reader.Serving, interfaces.ResolvedInterface) {
	base, impl, _ := interfaceFixtureState(t)
	return base, impl
}

func interfaceFixtureState(t *testing.T) (*reader.Serving, interfaces.ResolvedInterface, testkit.Setup) {
	t.Helper()
	s := testkit.NewSetup(t, "")
	implSchema := "schema/implements.prefix/v1"
	operations := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(implSchema)}, Value: map[string]any{"entity": "Object", "aspect": "interfaces", "pattern": "keyed_collection", "additionalProperties": false, "fields": map[string]any{"interfaceRef": map[string]any{"type": "string", "required": true}, "prefix": map[string]any{"type": "string", "required": true}}}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/text-input/v1"}, Value: map[string]any{"entity": "Input", "additionalProperties": false, "fields": map[string]any{"text": map[string]any{"type": "string", "required": true}}}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "interface/prefix/v1"}, Value: map[string]any{"name": "Prefix", "meaning": "Append an object-provided prefix to text.", "standardImplementationSchemas": []any{implSchema}, "operations": map[string]any{"read": map[string]any{"inputSchemaRef": "schema/text-input/v1", "resultSchemaRef": "schema/text-input/v1", "effect": "read"}, "write": map[string]any{"effect": "write"}}}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "object/message", AspectName: "interfaces", MemberKey: "friendly"}, SchemaRef: implSchema, Value: map[string]any{"interfaceRef": "interface/prefix/v1", "prefix": "Hello "}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "object/message", AspectName: "interfaces", MemberKey: "formal"}, SchemaRef: implSchema, Value: map[string]any{"interfaceRef": "interface/prefix/v1", "prefix": "Dear "}},
	}
	head, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: head}
	base := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) { return s.Repo, nil }, reader.DatasetPin{SetID: "test", Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
	impl, err := interfaces.ResolveObjectInterfaceAt(base, s.RepositoryID, "object/message", "friendly")
	if err != nil {
		t.Fatal(err)
	}
	return base, impl, s
}

func TestIndependentObjectInterfaceDispatch(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{}
	registry := interfaces.NewInterfaceRegistry()
	if err := registry.Register(interfaces.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "prefix-handler-v1", Handler: handler}); err != nil {
		t.Fatal(err)
	}
	service := interfaceservice.Open(base, registry, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	result, err := service.Call(context.Background(), interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.(map[string]any)["text"] != "Hello World" || handler.calls != 1 || result.Basis.DeclarationCommit != impl.Commit || result.Basis.HandlerRevision != "prefix-handler-v1" {
		t.Fatalf("result %#v calls %d", result, handler.calls)
	}
	_, err = service.Call(context.Background(), interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": 12}})
	if kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid || handler.calls != 1 {
		t.Fatalf("invalid input executed handler: %v calls=%d", err, handler.calls)
	}
}

func TestObjectInterfaceDiscoveryAndNoImplicitHandler(t *testing.T) {
	base, impl := interfaceFixture(t)
	implementations, err := interfaces.DiscoverObjectInterfaces(base, "object/message")
	if err != nil || len(implementations) != 2 {
		t.Fatalf("implementations %#v error %v", implementations, err)
	}
	registry := interfaces.NewInterfaceRegistry()
	service := interfaceservice.Open(base, registry, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	_, err = service.Call(context.Background(), interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing handler: %v", err)
	}
}

func registerPrefix(t *testing.T, impl interfaces.ResolvedInterface, handler *prefixInterfaceHandler) *interfaces.InterfaceRegistry {
	t.Helper()
	registry := interfaces.NewInterfaceRegistry()
	err := registry.Register(interfaces.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "prefix-v1", Handler: handler})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestObjectInterfaceKeepsHistoricalContractAfterHeadAdvance(t *testing.T) {
	base, impl, state := interfaceFixtureState(t)
	_, err := state.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: state.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: impl.Commit, ExpectedTargetCommit: impl.Commit, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: impl.Contract.Object}, Value: map[string]any{"name": "Changed", "meaning": "A newer incompatible contract.", "operations": map[string]any{"other": map[string]any{"effect": "read"}}}},
		{Op: objectmodel.OpPut, Address: impl.Address, SchemaRef: string(impl.ImplementationSchema.Object), Value: map[string]any{"interfaceRef": string(impl.Contract.Object), "prefix": "New "}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler := &prefixInterfaceHandler{}
	service := interfaceservice.Open(base, registerPrefix(t, impl, handler), objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	result, err := service.Call(context.Background(), interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if err != nil || result.Value.(map[string]any)["text"] != "Hello World" || result.Basis.Contract != impl.Contract || result.Basis.ContractDigest != impl.ContractDigest {
		t.Fatalf("historical drift: %#v %v", result, err)
	}
}

func TestObjectInterfaceOperationAuthorizationAndOutputValidation(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{invalid: true}
	service := interfaceservice.Open(base, registerPrefix(t, impl, handler), objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	request := interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "write", Input: map[string]any{"text": "World"}}
	_, err := service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 0 {
		t.Fatalf("write bypassed authorization: %v calls %d", err, handler.calls)
	}
	service.SetAuthorizer(func(context.Context, interfaces.InterfaceCall) error {
		return kernel.Fail(kernel.ErrScopeDenied, "denied")
	})
	_, err = service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrScopeDenied || handler.calls != 0 {
		t.Fatalf("write ignored denial: %v calls %d", err, handler.calls)
	}
	service.SetAuthorizer(func(context.Context, interfaces.InterfaceCall) error { return nil })
	request.Operation = "read"
	_, err = service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid || handler.calls != 1 {
		t.Fatalf("invalid output accepted: %v calls %d", err, handler.calls)
	}
	request.Operation = "undeclared"
	_, err = service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 1 {
		t.Fatalf("undeclared operation executed: %v calls %d", err, handler.calls)
	}
}

func TestObjectInterfaceRegistrationRejectsAmbiguityAndWrongBasis(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{}
	registry := registerPrefix(t, impl, handler)
	err := registry.Register(interfaces.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "other-v1", Handler: &prefixInterfaceHandler{}})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("ambiguous registration: %v", err)
	}
	registry = interfaces.NewInterfaceRegistry()
	err = registry.Register(interfaces.InterfaceRegistration{Contract: impl.Contract, ContractDigest: "wrong-contract", ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "prefix-v1", Handler: handler})
	if err != nil {
		t.Fatal(err)
	}
	service := interfaceservice.Open(base, registry, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	_, err = service.Call(context.Background(), interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 0 {
		t.Fatalf("wrong contract support accepted: %v calls %d", err, handler.calls)
	}
}

func TestObjectInterfaceDependenciesCannotEscapeDatasetSlice(t *testing.T) {
	base, impl := interfaceFixture(t)
	pin := base.Pin()
	repo, err := base.Member(impl.Repository)
	if err != nil {
		t.Fatal(err)
	}
	locator := repo.(objectmodel.UnitLocator)
	paths, err := locator.ObjectUnitPaths(impl.Address.ObjectID, impl.Commit)
	if err != nil {
		t.Fatal(err)
	}
	pin.Items = nil
	for _, path := range paths {
		pin.Items = append(pin.Items, reader.DatasetItem{Repository: impl.Repository, Commit: impl.Commit, Kind: "file", File: path})
	}
	slice := reader.Open(base.Member, pin)
	_, err = interfaces.ResolveObjectInterfaceAt(slice, impl.Repository, impl.Address.ObjectID, "friendly")
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("secondary schema read escaped slice: %v", err)
	}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	delete(pin.Repositories, impl.Repository)
	empty := reader.Open(base.Member, pin)
	_, err = interfaces.ResolveObjectInterfaceAt(empty, impl.Repository, impl.Address.ObjectID, "friendly")
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("primary read escaped pin: %v", err)
	}
}

func TestObjectInterfaceRequestedCapabilityMustMatch(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{}
	service := interfaceservice.Open(base, registerPrefix(t, impl, handler), objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	request := interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", InterfaceRef: "interface/another/v1", Operation: "read", Input: map[string]any{"text": "World"}}
	_, err := service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 0 {
		t.Fatalf("unexpected interface selected: %v calls=%d", err, handler.calls)
	}
	request.InterfaceRef = objectmodel.FormatPinnedRef(impl.Contract.Repository, impl.Contract.Commit, impl.Contract.Object)
	_, err = service.Call(context.Background(), request)
	if err != nil || handler.calls != 1 {
		t.Fatalf("explicit contract rejected: %v calls=%d", err, handler.calls)
	}
}

type mutatingInterfaceHandler struct{ t *testing.T }

func (h *mutatingInterfaceHandler) ValidateImplementation(_ context.Context, impl interfaces.ResolvedInterface) error {
	impl.Implementation["prefix"] = "validation mutated "
	impl.Definition.Operations["read"] = interfaces.InterfaceOperation{Effect: "write"}
	return nil
}
func (h *mutatingInterfaceHandler) Execute(_ context.Context, call interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
	if call.Implementation.Implementation["prefix"] != "Hello " || call.Input.(map[string]any)["text"] != "World" || call.Implementation.Definition.Operations["read"].Effect != "read" {
		h.t.Fatalf("earlier callback changed execution: %#v", call)
	}
	call.Implementation.Implementation["prefix"] = "execution mutated "
	call.Input.(map[string]any)["text"] = "execution changed input"
	return interfaces.InterfaceResult{Value: map[string]any{"text": "World"}}, nil
}

func TestObjectInterfaceCallbacksCannotMutatePinnedValuesOrCallerInput(t *testing.T) {
	base, impl := interfaceFixture(t)
	registry := interfaces.NewInterfaceRegistry()
	err := registry.Register(interfaces.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "mutator-v1", Handler: &mutatingInterfaceHandler{t: t}})
	if err != nil {
		t.Fatal(err)
	}
	service := interfaceservice.Open(base, registry, objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	service.SetAuthorizer(func(_ context.Context, call interfaces.InterfaceCall) error {
		call.Input.(map[string]any)["text"] = "authorization mutated input"
		call.Implementation.Implementation["prefix"] = "authorization mutated "
		return nil
	})
	input := map[string]any{"text": "World"}
	result, err := service.Call(context.Background(), interfaces.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: input})
	if err != nil || input["text"] != "World" || result.Basis.Contract != impl.Contract || result.Basis.ContractDigest != impl.ContractDigest {
		t.Fatalf("mutation escaped callback: %#v %#v %v", input, result, err)
	}
	after, err := interfaces.ResolveObjectInterfaceAt(base, impl.Repository, impl.Address.ObjectID, "friendly")
	if err != nil || after.Implementation["prefix"] != "Hello " || after.ContractDigest != impl.ContractDigest {
		t.Fatalf("mutation changed pinned declaration: %#v %v", after, err)
	}
	var absent *interfaces.InterfaceRegistry
	err = absent.Register(interfaces.InterfaceRegistration{})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("nil registry did not fail closed: %v", err)
	}
}

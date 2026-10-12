package serving_test

import (
	"context"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/snapshot"
)

type prefixInterfaceHandler struct {
	calls   int
	invalid bool
}

func (h *prefixInterfaceHandler) ValidateImplementation(_ context.Context, impl reader.ResolvedInterface) error {
	if _, ok := impl.Implementation["prefix"].(string); !ok {
		return kernel.Fail(kernel.ErrSchemaInstanceInvalid, "missing prefix")
	}
	return nil
}
func (h *prefixInterfaceHandler) Execute(_ context.Context, call serving.InterfaceCall) (serving.InterfaceResult, error) {
	h.calls++
	if h.invalid {
		return serving.InterfaceResult{Value: map[string]any{"text": 42}}, nil
	}
	return serving.InterfaceResult{Value: map[string]any{"text": call.Implementation.Implementation["prefix"].(string) + call.Input.(map[string]any)["text"].(string)}}, nil
}

func interfaceFixture(t *testing.T) (*reader.Serving, reader.ResolvedInterface) {
	base, impl, _ := interfaceFixtureState(t)
	return base, impl
}

func interfaceFixtureState(t *testing.T) (*reader.Serving, reader.ResolvedInterface, testkit.Setup) {
	t.Helper()
	s := testkit.NewSetup(t, "")
	implSchema := "schema/implements.prefix/v1"
	operations := []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(implSchema)}, Value: map[string]any{"entity": "Object", "aspect": "interfaces", "pattern": "keyed_collection", "additionalProperties": false, "fields": map[string]any{"interfaceRef": map[string]any{"type": "string", "required": true}, "prefix": map[string]any{"type": "string", "required": true}}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/text-input/v1"}, Value: map[string]any{"entity": "Input", "additionalProperties": false, "fields": map[string]any{"text": map[string]any{"type": "string", "required": true}}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "interface/prefix/v1"}, Value: map[string]any{"name": "Prefix", "meaning": "Append an object-provided prefix to text.", "standardImplementationSchemas": []any{implSchema}, "operations": map[string]any{"read": map[string]any{"inputSchemaRef": "schema/text-input/v1", "resultSchemaRef": "schema/text-input/v1", "effect": "read"}, "write": map[string]any{"effect": "write"}}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "object/message", AspectName: "interfaces", MemberKey: "friendly"}, SchemaRef: implSchema, Value: map[string]any{"interfaceRef": "interface/prefix/v1", "prefix": "Hello "}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "object/message", AspectName: "interfaces", MemberKey: "formal"}, SchemaRef: implSchema, Value: map[string]any{"interfaceRef": "interface/prefix/v1", "prefix": "Dear "}},
	}
	head, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	repositories := map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: head}
	base := reader.Open(func(id kernel.RepositoryID) (knowledge.Repository, error) { return s.Repo, nil }, reader.KnowledgeSetPin{SetID: "test", Repositories: repositories, Items: reader.WholeRepositoryItems(repositories)})
	impl, err := base.ResolveObjectInterfaceAt(s.RepositoryID, "object/message", "friendly")
	if err != nil {
		t.Fatal(err)
	}
	return base, impl, s
}

func TestIndependentObjectInterfaceDispatch(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{}
	registry := serving.NewInterfaceRegistry()
	if err := registry.Register(serving.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "prefix-handler-v1", Handler: handler}); err != nil {
		t.Fatal(err)
	}
	service := serving.OpenInterfaces(base, registry, serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	result, err := service.Call(context.Background(), serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.(map[string]any)["text"] != "Hello World" || handler.calls != 1 || result.Basis.DeclarationCommit != impl.Commit || result.Basis.HandlerRevision != "prefix-handler-v1" {
		t.Fatalf("result %#v calls %d", result, handler.calls)
	}
	_, err = service.Call(context.Background(), serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": 12}})
	if kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid || handler.calls != 1 {
		t.Fatalf("invalid input executed handler: %v calls=%d", err, handler.calls)
	}
}

func TestObjectInterfaceDiscoveryAndNoImplicitHandler(t *testing.T) {
	base, impl := interfaceFixture(t)
	implementations, err := base.DiscoverObjectInterfaces("object/message")
	if err != nil || len(implementations) != 2 {
		t.Fatalf("implementations %#v error %v", implementations, err)
	}
	registry := serving.NewInterfaceRegistry()
	service := serving.OpenInterfaces(base, registry, serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	_, err = service.Call(context.Background(), serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing handler: %v", err)
	}
}

func registerPrefix(t *testing.T, impl reader.ResolvedInterface, handler *prefixInterfaceHandler) *serving.InterfaceRegistry {
	t.Helper()
	registry := serving.NewInterfaceRegistry()
	err := registry.Register(serving.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "prefix-v1", Handler: handler})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestObjectInterfaceKeepsHistoricalContractAfterHeadAdvance(t *testing.T) {
	base, impl, state := interfaceFixtureState(t)
	_, err := state.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: state.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: impl.Commit, ExpectedTargetCommit: impl.Commit, Operations: []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: impl.Contract.Object}, Value: map[string]any{"name": "Changed", "meaning": "A newer incompatible contract.", "operations": map[string]any{"other": map[string]any{"effect": "read"}}}},
		{Op: knowledge.OpPut, Address: impl.Address, SchemaRef: string(impl.ImplementationSchema.Object), Value: map[string]any{"interfaceRef": string(impl.Contract.Object), "prefix": "New "}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler := &prefixInterfaceHandler{}
	service := serving.OpenInterfaces(base, registerPrefix(t, impl, handler), serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	result, err := service.Call(context.Background(), serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
	if err != nil || result.Value.(map[string]any)["text"] != "Hello World" || result.Basis.Contract != impl.Contract || result.Basis.ContractDigest != impl.ContractDigest {
		t.Fatalf("historical drift: %#v %v", result, err)
	}
}

func TestObjectInterfaceOperationAuthorizationAndOutputValidation(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{invalid: true}
	service := serving.OpenInterfaces(base, registerPrefix(t, impl, handler), serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	request := serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "write", Input: map[string]any{"text": "World"}}
	_, err := service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 0 {
		t.Fatalf("write bypassed authorization: %v calls %d", err, handler.calls)
	}
	service.SetAuthorizer(func(context.Context, serving.InterfaceCall) error {
		return kernel.Fail(kernel.ErrScopeDenied, "denied")
	})
	_, err = service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrScopeDenied || handler.calls != 0 {
		t.Fatalf("write ignored denial: %v calls %d", err, handler.calls)
	}
	service.SetAuthorizer(func(context.Context, serving.InterfaceCall) error { return nil })
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
	err := registry.Register(serving.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "other-v1", Handler: &prefixInterfaceHandler{}})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("ambiguous registration: %v", err)
	}
	registry = serving.NewInterfaceRegistry()
	err = registry.Register(serving.InterfaceRegistration{Contract: impl.Contract, ContractDigest: "wrong-contract", ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "prefix-v1", Handler: handler})
	if err != nil {
		t.Fatal(err)
	}
	service := serving.OpenInterfaces(base, registry, serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	_, err = service.Call(context.Background(), serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: map[string]any{"text": "World"}})
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
	locator := repo.(knowledge.UnitLocator)
	paths, err := locator.ObjectUnitPaths(impl.Address.ObjectID, impl.Commit)
	if err != nil {
		t.Fatal(err)
	}
	pin.Items = nil
	for _, path := range paths {
		pin.Items = append(pin.Items, reader.DatasetItem{Repository: impl.Repository, Commit: impl.Commit, Kind: "file", File: path})
	}
	slice := reader.Open(base.Member, pin)
	_, err = slice.ResolveObjectInterfaceAt(impl.Repository, impl.Address.ObjectID, "friendly")
	if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("secondary schema read escaped slice: %v", err)
	}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	delete(pin.Repositories, impl.Repository)
	empty := reader.Open(base.Member, pin)
	_, err = empty.ResolveObjectInterfaceAt(impl.Repository, impl.Address.ObjectID, "friendly")
	if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("primary read escaped pin: %v", err)
	}
}

func TestObjectInterfaceRequestedCapabilityMustMatch(t *testing.T) {
	base, impl := interfaceFixture(t)
	handler := &prefixInterfaceHandler{}
	service := serving.OpenInterfaces(base, registerPrefix(t, impl, handler), serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	request := serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", InterfaceRef: "interface/another/v1", Operation: "read", Input: map[string]any{"text": "World"}}
	_, err := service.Call(context.Background(), request)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || handler.calls != 0 {
		t.Fatalf("unexpected interface selected: %v calls=%d", err, handler.calls)
	}
	request.InterfaceRef = knowledge.FormatPinnedRef(impl.Contract.Repository, impl.Contract.Commit, impl.Contract.Object)
	_, err = service.Call(context.Background(), request)
	if err != nil || handler.calls != 1 {
		t.Fatalf("explicit contract rejected: %v calls=%d", err, handler.calls)
	}
}

type mutatingInterfaceHandler struct{ t *testing.T }

func (h *mutatingInterfaceHandler) ValidateImplementation(_ context.Context, impl reader.ResolvedInterface) error {
	impl.Implementation["prefix"] = "validation mutated "
	impl.Definition.Operations["read"] = knowledge.InterfaceOperation{Effect: "write"}
	return nil
}
func (h *mutatingInterfaceHandler) Execute(_ context.Context, call serving.InterfaceCall) (serving.InterfaceResult, error) {
	if call.Implementation.Implementation["prefix"] != "Hello " || call.Input.(map[string]any)["text"] != "World" || call.Implementation.Definition.Operations["read"].Effect != "read" {
		h.t.Fatalf("earlier callback changed execution: %#v", call)
	}
	call.Implementation.Implementation["prefix"] = "execution mutated "
	call.Input.(map[string]any)["text"] = "execution changed input"
	return serving.InterfaceResult{Value: map[string]any{"text": "World"}}, nil
}

func TestObjectInterfaceCallbacksCannotMutatePinnedValuesOrCallerInput(t *testing.T) {
	base, impl := interfaceFixture(t)
	registry := serving.NewInterfaceRegistry()
	err := registry.Register(serving.InterfaceRegistration{Contract: impl.Contract, ContractDigest: impl.ContractDigest, ImplementationSchemaStructureDigest: impl.ImplementationSchemaStructureDigest, HandlerRevision: "mutator-v1", Handler: &mutatingInterfaceHandler{t: t}})
	if err != nil {
		t.Fatal(err)
	}
	service := serving.OpenInterfaces(base, registry, serving.RequestContext{Identity: kernel.IdentityContext{Principal: "agent"}})
	service.SetAuthorizer(func(_ context.Context, call serving.InterfaceCall) error {
		call.Input.(map[string]any)["text"] = "authorization mutated input"
		call.Implementation.Implementation["prefix"] = "authorization mutated "
		return nil
	})
	input := map[string]any{"text": "World"}
	result, err := service.Call(context.Background(), serving.InterfaceRequest{Repository: impl.Repository, Object: impl.Address.ObjectID, Implementation: "friendly", Operation: "read", Input: input})
	if err != nil || input["text"] != "World" || result.Basis.Contract != impl.Contract || result.Basis.ContractDigest != impl.ContractDigest {
		t.Fatalf("mutation escaped callback: %#v %#v %v", input, result, err)
	}
	after, err := base.ResolveObjectInterfaceAt(impl.Repository, impl.Address.ObjectID, "friendly")
	if err != nil || after.Implementation["prefix"] != "Hello " || after.ContractDigest != impl.ContractDigest {
		t.Fatalf("mutation changed pinned declaration: %#v %v", after, err)
	}
	var absent *serving.InterfaceRegistry
	err = absent.Register(serving.InterfaceRegistration{})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("nil registry did not fail closed: %v", err)
	}
}

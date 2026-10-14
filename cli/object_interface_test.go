package cli_test

import (
	"context"
	"encoding/json"
	"kc/interfaces"
	liveaccesscontract "kc/liveaccess/contract"
	livefixture "kc/liveaccess/fixture"
	"kc/systemrepo"
	"net/http"
	"net/http/httptest"
	"testing"

	apphome "kc/home"
	"kc/internal/testkit"
	"kc/invocation"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/observability"
	serv "kc/server"
)

type echoObjectInterface struct{ calls int }

func (h *echoObjectInterface) ValidateImplementation(context.Context, interfaces.ResolvedInterface) error {
	return nil
}

func TestPublicInvokeLiveAccessUsesPublishedContract(t *testing.T) {
	homeDir := testkit.TempDir(t)
	repositoryID := "kr://test/live-access"
	body(t, kc(homeDir, "init"))
	seedRepo(t, homeDir, repositoryID)
	body(t, kc(homeDir, "grant", "add", "--principal", "reader", "--action", "resource.access", "--repo", repositoryID))
	publishLiveAccessDefinitions(t, homeDir, repositoryID)
	var calls int
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Resource-Principal") != "reader" {
			t.Errorf("principal: %#v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"count": 8}, "basis": objectmodel.ObservationBasis{BindingGeneration: "table-runtime/v1", Consistency: objectmodel.ObservationBounded, SourceRevision: "rows-8", ObservedAt: "2026-10-10T03:00:00Z"}})
	}))
	defer runtime.Close()
	implementation, err := json.Marshal(map[string]any{"interfaceRef": string(liveaccesscontract.LiveAccessInterfaceV1), "runtime": "tables", "protocol": objectmodel.ProtocolResourceAccessV1, "origin": runtime.URL, "operations": map[string]any{"read": map[string]any{"call": "orders.state"}}})
	if err != nil {
		t.Fatal(err)
	}
	body(t, kc(homeDir, "put", testObjectReference(t, repositoryID, "table/orders", "/interfaces/live"), "--command-id", "live-implementation", "--schema-ref", string(liveaccesscontract.LiveAccessImplementationSchemaV1), "--value", string(implementation), "--kind", "Member"))
	serverHandler := serv.HTTPHandlerWithOptions(homeDir, serv.HTTPServerOptions{})
	if closer, ok := serverHandler.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	server := httptest.NewServer(serverHandler)
	defer server.Close()
	result := asMap(t, body(t, kcRemote(t, server.URL, "reader", "invoke", "--repo", repositoryID, "--object", "table/orders", "--implementation", "live", "--operation", "read", "--input", "{}")))
	if asMap(t, result["value"])["count"] != float64(8) || asMap(t, result["observation"])["sourceRevision"] != "rows-8" || asMap(t, result["basis"])["handlerRevision"] != "live-access-http/v1" || calls != 1 {
		t.Fatalf("LiveAccess public receipt: %#v calls=%d", result, calls)
	}
	assertInterfaceAccessEvidence(t, homeDir, repositoryID, "table/orders", "live", result)
}

func publishLiveAccessDefinitions(t *testing.T, homeDir, repositoryID string) {
	t.Helper()
	adoptLiveAccessFixture(t, homeDir, repositoryID)
	operations := systemrepo.SystemPublicationOperations()
	// Individual Writer requests require the Schema to exist before its
	// instances, independently of the complete publication bundle's order.
	for _, schemas := range []bool{true, false} {
		for _, operation := range operations {
			id := operation.Address.ObjectID
			if id != liveaccesscontract.LiveAccessImplementationSchemaV1 && id != liveaccesscontract.LiveAccessInterfaceV1 && id != "schema/core/object-interface/v1" {
				continue
			}
			if objectmodel.IsSchemaObject(id) != schemas {
				continue
			}
			raw, err := json.Marshal(operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			locator := ""
			if operation.Address.AspectName != "" {
				locator = testObjectLocator(operation.Address.AspectName)
				if operation.Address.MemberKey != "" {
					locator = testObjectLocator(operation.Address.AspectName, operation.Address.MemberKey)
				}
			}
			args := []string{"put", testObjectReference(t, repositoryID, string(id), locator), "--command-id", "live-publish-" + string(id), "--value", string(raw), "--kind", string(operation.Address.Kind)}
			if operation.SchemaRef != "" {
				args = append(args, "--schema-ref", operation.SchemaRef)
			}
			body(t, kc(homeDir, args...))
		}
	}
}

func adoptLiveAccessFixture(t *testing.T, directory, repository string) {
	t.Helper()
	file, err := apphome.ReadHome(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, declared := range file.Repos {
		if declared.ID == repository && declared.Driver == "lakefs" {
			if err := livefixture.AdoptRepository(kernel.RepositoryID(repository), declared.DSN, "main"); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("explicit LiveAccess fixture source %s has no declared lakeFS DSN", repository)
}
func (h *echoObjectInterface) Execute(_ context.Context, call interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
	h.calls++
	return interfaces.InterfaceResult{Value: map[string]any{"text": call.Implementation.Implementation["prefix"].(string) + call.Input.(map[string]any)["text"].(string)}}, nil
}

func TestPublicInvokeDispatchesIndependentObjectInterfaceAtFixedCommit(t *testing.T) {
	homeDir := testkit.TempDir(t)
	repositoryID := "kr://test/interfaces"
	body(t, kc(homeDir, "init"))
	seedRepo(t, homeDir, repositoryID)
	body(t, kc(homeDir, "grant", "add", "--principal", "maintainer", "--action", "resource.access", "--repo", repositoryID))
	values := []struct{ object, value string }{
		{"schema/echo-implementation/v1", `{"entity":"Object","aspect":"interfaces","pattern":"keyed_collection","fields":{"interfaceRef":{"type":"string","required":true},"prefix":{"type":"string","required":true}}}`},
		{"schema/echo-input/v1", `{"entity":"Input","fields":{"text":{"type":"string","required":true}}}`},
		{"interface/echo/v1", `{"name":"Echo","meaning":"An independently installed extension.","operations":{"read":{"effect":"read","inputSchemaRef":"schema/echo-input/v1","resultSchemaRef":"schema/echo-input/v1"}}}`},
	}
	for n, value := range values {
		body(t, kc(homeDir, "put", testObjectReference(t, repositoryID, value.object, ""), "--command-id", "echo-schema-"+string(rune('a'+n)), "--value", value.value))
	}
	body(t, kc(homeDir, "put", testObjectReference(t, repositoryID, "object/message", "/interfaces/echo"), "--command-id", "echo-implementation", "--schema-ref", "schema/echo-implementation/v1", "--value", `{"interfaceRef":"interface/echo/v1","prefix":"Old "}`, "--kind", "Member"))
	pinned := asMap(t, body(t, kc(homeDir, "read", testObjectReference(t, repositoryID, "object/message", ""))))["commit"].(string)
	opened, err := apphome.Open(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	implementation, err := interfaces.ResolveObjectInterface(opened.Reader, kernel.RepositoryID(repositoryID), kernel.CommitID(pinned), "object/message", "echo")
	if err != nil {
		t.Fatal(err)
	}
	handler := &echoObjectInterface{}
	registry := interfaces.NewInterfaceRegistry()
	if err := registry.Register(interfaces.InterfaceRegistration{Contract: implementation.Contract, ContractDigest: implementation.ContractDigest, ImplementationSchemaStructureDigest: implementation.ImplementationSchemaStructureDigest, HandlerRevision: "echo/v1", Handler: handler}); err != nil {
		t.Fatal(err)
	}
	body(t, kc(homeDir, "put", testObjectReference(t, repositoryID, "object/message", "/interfaces/echo"), "--command-id", "echo-new-head", "--schema-ref", "schema/echo-implementation/v1", "--value", `{"interfaceRef":"interface/echo/v1","prefix":"New "}`, "--kind", "Member"))
	publishLiveAccessDefinitions(t, homeDir, repositoryID)
	var liveCalls int
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		liveCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"count": 9}, "basis": objectmodel.ObservationBasis{BindingGeneration: "table-runtime/v1", Consistency: objectmodel.ObservationBounded, SourceRevision: "rows-9", ObservedAt: "2026-10-10T03:00:00Z"}})
	}))
	defer runtime.Close()
	live, err := json.Marshal(map[string]any{"interfaceRef": string(liveaccesscontract.LiveAccessInterfaceV1), "runtime": "tables", "protocol": objectmodel.ProtocolResourceAccessV1, "origin": runtime.URL, "operations": map[string]any{"read": map[string]any{"call": "orders.state"}}})
	if err != nil {
		t.Fatal(err)
	}
	body(t, kc(homeDir, "put", testObjectReference(t, repositoryID, "table/orders", "/interfaces/live"), "--command-id", "shared-live-implementation", "--schema-ref", string(liveaccesscontract.LiveAccessImplementationSchemaV1), "--value", string(live), "--kind", "Member"))
	serverHandler := serv.HTTPHandlerWithOptions(homeDir, serv.HTTPServerOptions{ObjectInterfaces: registry})
	if closer, ok := serverHandler.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	server := httptest.NewServer(serverHandler)
	defer server.Close()
	result := asMap(t, body(t, kcRemote(t, server.URL, "maintainer", "invoke", "--repo", repositoryID, "--commit", pinned, "--object", "object/message", "--implementation", "echo", "--operation", "read", "--input", `{"text":"World"}`)))
	if asMap(t, result["value"])["text"] != "Old World" || asMap(t, result["basis"])["declarationCommit"] != pinned || handler.calls != 1 {
		t.Fatalf("pinned public dispatch: %#v calls=%d", result, handler.calls)
	}
	assertInterfaceAccessEvidence(t, homeDir, repositoryID, "object/message", "echo", result)
	invalid := kcRemote(t, server.URL, "maintainer", "invoke", "--repo", repositoryID, "--commit", pinned, "--object", "object/message", "--implementation", "echo", "--operation", "read", "--input", `{"text":12}`)
	if invalid.Status == 0 || handler.calls != 1 {
		t.Fatalf("invalid input dispatched: %#v calls=%d", invalid, handler.calls)
	}
	unknown := kcRemote(t, server.URL, "maintainer", "invoke", "--repo", repositoryID, "--object", "object/message", "--implementation", "echo", "--operation", "read", "--input", `{"text":"World"}`)
	if unknown.Status == 0 || handler.calls != 1 {
		t.Fatalf("new release silently used old handler: %#v", unknown)
	}
	liveResult := asMap(t, body(t, kcRemote(t, server.URL, "maintainer", "invoke", "--repo", repositoryID, "--object", "table/orders", "--implementation", "live", "--operation", "read", "--input", "{}")))
	if asMap(t, liveResult["value"])["count"] != float64(9) || liveCalls != 1 {
		t.Fatalf("installing Echo removed built-in LiveAccess: %#v calls=%d", liveResult, liveCalls)
	}
	assertInterfaceAccessEvidence(t, homeDir, repositoryID, "table/orders", "live", liveResult)
	// The built-in request must not install a new handler in the shared registry.
	result = asMap(t, body(t, kcRemote(t, server.URL, "maintainer", "invoke", "--repo", repositoryID, "--commit", pinned, "--object", "object/message", "--implementation", "echo", "--operation", "read", "--input", `{"text":"Again"}`)))
	if asMap(t, result["value"])["text"] != "Old Again" || handler.calls != 2 {
		t.Fatalf("LiveAccess composition changed installed extension: %#v calls=%d", result, handler.calls)
	}
}

func assertInterfaceAccessEvidence(t *testing.T, homeDir, repository, object, member string, result map[string]any) {
	t.Helper()
	page, err := observability.NewFileStore(homeDir).Access(context.Background(), observability.AccessQuery{Action: "resource.access", Repository: kernel.RepositoryID(repository), Object: objectmodel.ObjectID(object)})
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("public call has no exact access evidence: %#v %v", page, err)
	}
	event := page.Entries[0]
	if len(event.Knowledge) != 3 || event.Knowledge[0].Address == nil || event.Knowledge[0].Address.MemberKey != member {
		t.Fatalf("public call dropped Member or dependency refs: %#v", event)
	}
	basis := asMap(t, result["basis"])
	if string(event.Knowledge[0].KnowledgeRef.Commit) != basis["declarationCommit"] || string(event.Knowledge[1].KnowledgeRef.Object) != asMap(t, basis["contract"])["object"] || string(event.Knowledge[2].KnowledgeRef.Object) != asMap(t, basis["implementationSchema"])["object"] {
		t.Fatalf("public evidence differs from execution basis: %#v %#v", event, basis)
	}
	if observation, ok := result["observation"].(map[string]any); ok {
		if len(event.Knowledge[0].Observations) != 1 || event.Knowledge[0].Observations[0].Basis.SourceRevision != observation["sourceRevision"] {
			t.Fatalf("public read dropped actual observation: %#v", event)
		}
	}
	trail, err := invocation.ReadTrail(homeDir, "", "resource.access", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range trail {
		if entry.EvidenceID == event.EvidenceID {
			if entry.Refs["handlerRevision"] != basis["handlerRevision"] || entry.Refs["contractDigest"] != basis["contractDigest"] || entry.Refs["implementationSchemaDigest"] != basis["implementationSchemaDigest"] {
				t.Fatalf("public audit dropped execution basis: %#v", entry)
			}
			return
		}
	}
	t.Fatalf("public access receipt is not linked from audit: %#v", event)
}

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/observability"
	"kc/retrieval"
	"kc/retrieval/opensearch"
	"kc/snapshot"
)

func TestHTTPStateLookupCallsIndependentResourceRuntime(t *testing.T) {
	var got stateRuntimeRequest
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/access" || r.Method != http.MethodPost {
			t.Fatalf("runtime request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Resource-Principal") != "agent" || r.Header.Get("X-Resource-On-Behalf-Of") != "alice" || r.Header.Get("X-Resource-Request-Id") != "req-7" {
			t.Fatalf("runtime headers: %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"value": map[string]any{"status": "healthy"},
			"basis": map[string]any{
				"bindingGeneration": "runtime-v4", "consistency": "repeatable",
				"sourceRevision": "health-19", "observedAt": "2026-08-27T12:00:00Z",
			},
		})
	}))
	defer runtime.Close()
	lookup, err := NewHTTPStateLookup(runtime.URL, runtime.Client())
	if err != nil {
		t.Fatal(err)
	}
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	result, err := lookup.LookupState(context.Background(), serving.StateLookupRequest{
		Binding: reader.ResolvedBinding{
			Repository: "kr://acme/core", DeclarationCommit: "c1", Address: address,
			DeclarationDigest: "decl-1", DescriptorRef: "resource/health", DescriptorDigest: "descriptor-1",
			Mode: knowledge.BindingState, Runtime: "health-runtime", Protocol: "resource-access/v1",
			Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "health.lookup"}},
		},
		SchemaRef: "schema/health", Identity: observability.IdentityContext{Principal: "agent", OnBehalfOf: "alice"},
		Trace: observability.TraceContext{TraceID: "trace-1", SpanID: "span-1"}, RequestID: "req-7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.(map[string]any)["status"] != "healthy" || result.Basis.SourceRevision != "health-19" {
		t.Fatalf("result: %#v", result)
	}
	if got.Operation != "lookup" || got.Call != "health.lookup" || got.Runtime != "health-runtime" || got.SchemaRef != "schema/health" {
		t.Fatalf("runtime payload: %#v", got)
	}
	if got.Binding.DeclarationCommit != "c1" || got.Binding.DeclarationDigest != "decl-1" || got.Binding.DescriptorDigest != "descriptor-1" {
		t.Fatalf("runtime binding basis: %#v", got.Binding)
	}
	if got.Identity.TraceID != "trace-1" || got.Identity.RequestID != "req-7" {
		t.Fatalf("runtime identity/correlation: %#v", got.Identity)
	}
}

func TestHTTPStateLookupRejectsUnsupportedAndDishonestRuntime(t *testing.T) {
	lookup, err := NewHTTPStateLookup("https://runtime.example/base", &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lookup.LookupState(context.Background(), serving.StateLookupRequest{Binding: reader.ResolvedBinding{
		Address:    knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:x", AspectName: "health"},
		Operations: map[string]knowledge.BindingOperation{"window": {Call: "health.window"}},
	}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing lookup/read operation: %v", err)
	}

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"status": "healthy"}})
	}))
	defer runtime.Close()
	lookup, err = NewHTTPStateLookup(runtime.URL, runtime.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = lookup.LookupState(context.Background(), serving.StateLookupRequest{Binding: reader.ResolvedBinding{
		Address:    knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:x", AspectName: "health"},
		Operations: map[string]knowledge.BindingOperation{"read": {Call: "health.read"}},
	}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("response without value/basis must fail capability: %v", err)
	}
}

func TestNewHTTPStateLookupValidatesServiceOrigin(t *testing.T) {
	for _, raw := range []string{"", "file:///tmp/runtime", "https://user:secret@example.com", "https://runtime.example/v1/access", "https://runtime.example?token=x"} {
		if _, err := NewHTTPStateLookup(raw, nil); err == nil {
			t.Fatalf("accepted invalid runtime origin %q", raw)
		}
	}
}

func TestLiveHTTPStateRuntimeContainer(t *testing.T) {
	origin := os.Getenv("KC_TEST_STATE_RUNTIME_URL")
	if origin == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_STATE_RUNTIME_URL is required")
		}
		t.Skip("set KC_TEST_STATE_RUNTIME_URL or run make test-state-runtime-e2e")
	}
	lookup, err := NewHTTPStateLookup(origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := lookup.LookupState(context.Background(), serving.StateLookupRequest{
		Binding: reader.ResolvedBinding{
			Repository: "kr://acme/core", DeclarationCommit: "commit-live-1",
			Address:           knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"},
			DeclarationDigest: "decl-live-1", Mode: knowledge.BindingState,
			Runtime: "health", Protocol: "resource-access/v1",
			Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "health.lookup"}},
		},
		Identity: observability.IdentityContext{Principal: "agent:docker-test"}, RequestID: "docker-request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	value := result.Value.(map[string]any)
	if value["runtime"] != "docker" || result.Basis.BindingGeneration != "docker-runtime-v1" || result.Basis.SourceRevision != "docker-health-1" {
		t.Fatalf("container runtime result: %#v", result)
	}
}

func TestLiveHTTPRuntimeBuildsOpenSearchStateProjection(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_TEST_STATE_RUNTIME_URL"))
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if origin == "" || opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_STATE_RUNTIME_URL and KC_TEST_OPENSEARCH_URL are required")
		}
		t.Skip("run make test-state-runtime-e2e")
	}
	lookup, err := NewHTTPStateLookup(origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	setup := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	commit, err := setup.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health", "fields": map[string]any{
					"status": map[string]any{"type": "string", "access": []any{"text", "filter"}},
				},
			}},
			{Op: knowledge.OpPut, Address: address, Value: nil, SchemaRef: "schema/service.health", ValueSource: &knowledge.ValueSource{
				Kind:    knowledge.ValueSourceBinding,
				Binding: &knowledge.BindingDeclaration{Mode: knowledge.BindingState, Runtime: "health", Protocol: "resource-access/v1", Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "health.lookup"}}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewIndexEngine("", opensearch.Open(opensearch.Config{URL: opensearchURL, PrimaryShards: 1}))
	t.Cleanup(func() { _ = idx.Close() })
	sync, err := idx.RefreshState(context.Background(), setup.Repo, commit, lookup, serving.RequestContext{
		Identity: observability.IdentityContext{Principal: "agent:docker-test"}, RequestID: "docker-projection-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := idx.SearchStateAt(setup.Repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("healthy")))
	if err != nil || len(result.Hits) != 1 || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("cross-container State search: %#v %v", result, err)
	}
	if result.SearchView.ProjectionRevisions[setup.RepositoryID] != sync.Revision {
		t.Fatalf("SearchView revision: %#v", result.SearchView)
	}
	raw, err := setup.Repo.ReadAddress(address, commit)
	if err != nil || raw.Value != nil {
		t.Fatalf("runtime observation leaked into Snapshot: %#v %v", raw, err)
	}
	if head, err := setup.Repo.Head(snapshot.DefaultRef); err != nil || head != commit {
		t.Fatalf("dynamic refresh moved Repository HEAD: %s %v", head, err)
	}
}

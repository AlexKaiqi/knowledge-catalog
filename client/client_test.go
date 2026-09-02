package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"kc/cli"
	"kc/client"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

func TestPassThroughLoginCarriesIdentityAuthenticationAndTrace(t *testing.T) {
	seen := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seen <- request.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"principal":"agent:catalog","onBehalfOf":"user:kai"}`)
	}))
	t.Cleanup(server.Close)

	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := kc.Login(context.Background(), client.LoginRequest{
		Identity:       client.Identity{Principal: "agent:catalog", OnBehalfOf: "user:kai"},
		Authentication: client.Authentication{Authorization: "Bearer opaque-token"},
	})
	if err != nil || identity.Principal != "agent:catalog" {
		t.Fatalf("login: identity=%#v err=%v", identity, err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		Remote:     false,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	if _, err := kc.IdentityService().WhoAmI(ctx, client.RequestOptions{RequestID: "request-1"}); err != nil {
		t.Fatal(err)
	}
	header := <-seen
	if header.Get("Authorization") != "Bearer opaque-token" || header.Get("X-Kc-As") != "agent:catalog" || header.Get("X-Kc-On-Behalf-Of") != "user:kai" {
		t.Fatalf("identity/authentication not propagated: %#v", header)
	}
	if header.Get("X-Kc-Request-Id") != "request-1" || !strings.HasPrefix(header.Get("Traceparent"), "00-") {
		t.Fatalf("request/trace context not propagated: %#v", header)
	}
	if header.Get("Baggage") != "" {
		t.Fatalf("identity or credential must not enter baggage: %q", header.Get("Baggage"))
	}
}

func TestAuthDiscoveryDoesNotRequireLogin(t *testing.T) {
	var sawAuth, sawAs string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/identity/v1/auth" {
			http.NotFound(w, request)
			return
		}
		sawAuth = request.Header.Get("Authorization")
		sawAs = request.Header.Get("X-Kc-As")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mode": "local", "localAssertion": true, "accepts": []string{"X-Kc-As"},
		})
	}))
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := kc.IdentityService().Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "" || sawAs != "" {
		t.Fatalf("discovery sent credentials: Authorization=%q X-Kc-As=%q", sawAuth, sawAs)
	}
	if discovery.Mode != "local" || !discovery.LocalAssertion {
		t.Fatalf("discovery %#v", discovery)
	}
}

func TestLogoutClearsCredentialsBeforeNextCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "user:alice"}}); err != nil {
		t.Fatal(err)
	}
	if err := kc.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := kc.Identity(context.Background()); err != nil || ok {
		t.Fatalf("identity remains after logout: ok=%v err=%v", ok, err)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Do(context.Background(), "external-system", request); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("logged-out call: %v", err)
	}
}

func TestAuthenticationIsNotJSONSerialized(t *testing.T) {
	raw, err := json.Marshal(client.Session{
		Identity:       client.Identity{Principal: "user:alice"},
		Authentication: client.Authentication{Authorization: "Bearer secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "Authorization") {
		t.Fatalf("credential escaped through JSON: %s", raw)
	}
}

func TestKnowledgeObjectRequestOmitsZeroLimit(t *testing.T) {
	raw, err := json.Marshal(client.KnowledgeObjectRequest{
		Workspace: "agent", Object: "policy/A", Limit: 0, Continuation: "cursor-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, present := wire["limit"]; present {
		t.Fatalf("limit 0 must be omitted so the server applies the default page: %s", raw)
	}
	if wire["continuation"] != "cursor-1" || wire["object"] != "policy/A" {
		t.Fatalf("wire request = %#v", wire)
	}
}

func TestPagedKnowledgeRequestsOmitZeroLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"schema page", mustJSON(t, client.KnowledgeSchemaPageRequest{Repository: "kr://acme/public/core", Limit: 0})},
		{"relations", mustJSON(t, client.KnowledgeRelationsRequest{Workspace: "agent", Endpoint: "kc://acme/public/core/Table:x", Limit: 0})},
		{"search", mustJSON(t, client.KnowledgeSearchRequest{Workspace: "agent", Query: "refund", Limit: 0})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var wire map[string]any
			if err := json.Unmarshal(tc.raw, &wire); err != nil {
				t.Fatal(err)
			}
			if _, present := wire["limit"]; present {
				t.Fatalf("limit 0 must be omitted so the server applies the default page: %s", tc.raw)
			}
		})
	}
}

func TestKnowledgeSearchRequestSerializesExpressionAndOrder(t *testing.T) {
	expression := retrieval.SearchAny(
		retrieval.SearchLeaf(retrieval.SearchMATCH("payment")),
		retrieval.SearchLeaf(retrieval.SearchMATCH("database")),
	)
	order := retrieval.SearchSORT("severity", "asc")
	raw, err := json.Marshal(client.KnowledgeSearchRequest{
		Workspace: "agent", Expression: &expression, Order: &order, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire["expression"].(map[string]any)["any"].([]any)) != 2 || wire["order"].(map[string]any)["op"] != "SORT" {
		t.Fatalf("wire request = %#v", wire)
	}
}

func TestKnowledgeRerankRequestSerializesTypedCandidatesAndSpec(t *testing.T) {
	topK := 10
	raw, err := json.Marshal(client.KnowledgeRerankRequest{
		Workspace:  "agent",
		Candidates: []knowledge.KnowledgeRef{{Repository: "kr://acme/public/core", Object: "runbook/p1"}},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:runbook", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", OutputContract: retrieval.OutputContract{TopK: &topK},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["workspace"] != "agent" || wire["spec"].(map[string]any)["operator"] != "SEMANTIC_RERANK" || len(wire["candidates"].([]any)) != 1 {
		t.Fatalf("wire request = %#v", wire)
	}
}

func TestKnowledgeSearchRerankRequestSerializesBothStages(t *testing.T) {
	topK := 5
	raw, err := json.Marshal(client.KnowledgeSearchRerankRequest{
		KnowledgeSearchRequest: client.KnowledgeSearchRequest{Workspace: "agent", Query: "refund", Limit: 20},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:runbook", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "refund timeout relevance", OutputContract: retrieval.OutputContract{TopK: &topK},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["workspace"] != "agent" || wire["query"] != "refund" || wire["limit"] != float64(20) || wire["spec"].(map[string]any)["operator"] != "SEMANTIC_RERANK" {
		t.Fatalf("wire request = %#v", wire)
	}
}

func TestClientWorksWithLocalKCPassThroughServiceWithoutDelegation(t *testing.T) {
	home := t.TempDir()
	handler := cli.HTTPHandler(home)
	seenAs := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seenAs <- request.Header.Get("X-Kc-As")
		handler.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = kc.Login(context.Background(), client.LoginRequest{
		Identity: client.Identity{Principal: "agent:test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	who, err := kc.IdentityService().WhoAmI(context.Background(), client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if as := <-seenAs; as != "agent:test" {
		t.Fatalf("local pairing must send X-Kc-As: %q", as)
	}
	if who.Principal != "agent:test" || who.OnBehalfOf != "" {
		t.Fatalf("whoami: %#v", who)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

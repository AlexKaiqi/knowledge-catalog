package liveaccess

import (
	"kc/invoke"

	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/observability"
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
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tai-Identity") != "" {
			t.Fatalf("absent caller proof must not invent credentials: %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		invoke.WriteJSON(w, http.StatusOK, map[string]any{
			"value": map[string]any{"status": "healthy"},
			"basis": map[string]any{
				"bindingGeneration": "runtime-v4", "consistency": "repeatable",
				"sourceRevision": "health-19", "observedAt": "2026-08-27T12:00:00Z",
			},
		})
	}))
	defer runtime.Close()
	lookup := NewHTTPStateLookup(runtime.Client())
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	result, err := lookup.LookupState(context.Background(), StateLookupRequest{
		Binding: ResolvedBinding{
			Repository: "kr://acme/core", DeclarationCommit: "c1", Address: address,
			DeclarationDigest: "decl-1", DescriptorRef: "resource/health", DescriptorDigest: "descriptor-1",
			Mode: objectmodel.BindingState, Runtime: "health-runtime", Protocol: "resource-access/v1",
			Operations: map[string]objectmodel.BindingOperation{"lookup": {Call: "health.lookup"}},
			Origin:     runtime.URL,
		},
		SchemaRef: "schema/health", Origin: runtime.URL, Identity: observability.IdentityContext{Principal: "agent", OnBehalfOf: "alice"},
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
	lookup := NewHTTPStateLookup(&http.Client{})
	_, err := lookup.LookupState(context.Background(), StateLookupRequest{Binding: ResolvedBinding{
		Address:    objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/x", AspectName: "health"},
		Operations: map[string]objectmodel.BindingOperation{"window": {Call: "health.window"}},
		Origin:     "https://runtime.example/base",
	}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing lookup/read operation: %v", err)
	}

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		invoke.WriteJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"status": "healthy"}})
	}))
	defer runtime.Close()
	lookup = NewHTTPStateLookup(runtime.Client())
	_, err = lookup.LookupState(context.Background(), StateLookupRequest{
		Binding: ResolvedBinding{
			Address:    objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/x", AspectName: "health"},
			Operations: map[string]objectmodel.BindingOperation{"read": {Call: "health.read"}},
			Origin:     runtime.URL,
		},
		Identity: observability.IdentityContext{Principal: "agent"},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("response without value/basis must fail capability: %v", err)
	}
}

func TestHTTPStateLookupForwardsCallerAuthentication(t *testing.T) {
	var got stateRuntimeRequest
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer taihu-token" || r.Header.Get("X-Tai-Identity") != "signed.identity" {
			t.Fatalf("caller proof headers: %#v", r.Header)
		}
		if r.Header.Get("X-Resource-Principal") != "alice" {
			t.Fatalf("verified principal: %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		invoke.WriteJSON(w, http.StatusOK, map[string]any{
			"value": map[string]any{"status": "healthy"},
			"basis": map[string]any{
				"bindingGeneration": "runtime-v4", "consistency": "repeatable",
				"sourceRevision": "health-19", "observedAt": "2026-08-27T12:00:00Z",
			},
		})
	}))
	defer runtime.Close()
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: runtime.Client(), CredentialOrigins: []string{runtime.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{
		Authorization: "Bearer taihu-token", TaiIdentity: "signed.identity",
	})
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}
	if _, err := lookup.LookupState(ctx, StateLookupRequest{
		Binding: ResolvedBinding{
			Repository: "kr://acme/core", DeclarationCommit: "c1", Address: address,
			DeclarationDigest: "decl-1", Mode: objectmodel.BindingState, Protocol: "resource-access/v1",
			Operations: map[string]objectmodel.BindingOperation{"lookup": {Call: "health.lookup"}},
			Origin:     runtime.URL,
		},
		Origin: runtime.URL, Identity: observability.IdentityContext{Principal: "alice"},
	}); err != nil {
		t.Fatal(err)
	}
	if got.Identity.Principal != "alice" {
		t.Fatalf("identity JSON must carry principal: %#v", got.Identity)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "Bearer") || strings.Contains(string(body), "signed.identity") {
		t.Fatalf("caller proof leaked into JSON body: %s", body)
	}

	accessRuntime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer taihu-token" || r.Header.Get("X-Tai-Identity") != "signed.identity" {
			t.Fatalf("operation proof headers: %#v", r.Header)
		}
		invoke.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer accessRuntime.Close()
	accessLookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: accessRuntime.Client(), CredentialOrigins: []string{accessRuntime.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessLookup.AccessResource(ctx, ResourceOperationRequest{
		Origin: accessRuntime.URL, Runtime: "sql", Protocol: "resource-access/v1",
		Operation: "query", Call: "sql.query", Identity: StateRuntimeIdentity{Principal: "alice"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPStateLookupRequiresCallerPrincipal(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("resource-access must not be called without a principal")
	}))
	defer runtime.Close()
	lookup := NewHTTPStateLookup(runtime.Client())
	_, err := lookup.LookupState(context.Background(), StateLookupRequest{
		Binding: ResolvedBinding{
			Address:    objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/x", AspectName: "health"},
			Operations: map[string]objectmodel.BindingOperation{"lookup": {Call: "health.lookup"}},
			Origin:     runtime.URL,
		},
		Origin: runtime.URL,
	})
	if kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("missing principal: %v", err)
	}
}

func TestHTTPStateLookupRequiresDescriptorOrigin(t *testing.T) {
	lookup := NewHTTPStateLookup(nil)
	_, err := lookup.LookupState(context.Background(), StateLookupRequest{Binding: ResolvedBinding{
		Address:    objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/x", AspectName: "health"},
		Operations: map[string]objectmodel.BindingOperation{"lookup": {Call: "health.lookup"}},
	}})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing descriptor origin: %v", err)
	}
}

// A source that does not know the unit names that failure; it is a lookup
// failure with the source's code, never an empty value.
func TestHTTPStateLookupKeepsSourceFailureCode(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		code   kernel.ErrorCode
	}{
		{http.StatusNotFound, `{"error":{"code":"OBJECT_REF_UNRESOLVED","message":"unit is unknown at this source"}}`, kernel.ErrObjectRefUnresolved},
		{http.StatusNotFound, `{"error":{"message":"no such route"}}`, kernel.ErrCapabilityUnsatisfied},
		{http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, kernel.ErrTemporaryUnavailable},
		{http.StatusBadGateway, `{"error":{"code":"TEMPORARY_UNAVAILABLE","message":"upstream"}}`, kernel.ErrTemporaryUnavailable},
		{http.StatusForbidden, `{"error":{"code":"OBJECT_REF_UNRESOLVED","message":"denied"}}`, kernel.ErrForbidden},
	} {
		runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(test.status)
			_, _ = w.Write([]byte(test.body))
		}))
		lookup := NewHTTPStateLookup(runtime.Client())
		observation, err := lookup.LookupState(context.Background(), policyRequest(runtime.URL, "alice"))
		runtime.Close()
		if kernel.CodeOf(err) != test.code || observation.Value != nil {
			t.Fatalf("HTTP %d %s: got %v %#v, want %s", test.status, test.body, err, observation, test.code)
		}
	}
}

// An origin that refuses connections is a failed lookup for that unit, not a
// null value and not a transport panic of the whole read.
func TestHTTPStateLookupUnreachableOriginIsTemporaryFailure(t *testing.T) {
	runtime := httptest.NewServer(http.NotFoundHandler())
	address, client := runtime.URL, runtime.Client()
	runtime.Close()
	observation, err := NewHTTPStateLookup(client).LookupState(context.Background(), policyRequest(address, "alice"))
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || observation.Value != nil {
		t.Fatalf("unreachable origin = %v %#v, want TEMPORARY_UNAVAILABLE without value", err, observation)
	}
}

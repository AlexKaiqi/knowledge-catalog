package liveaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
)

func policyRequest(origin, principal string) StateLookupRequest {
	return StateLookupRequest{Binding: ResolvedBinding{Repository: "repo", DeclarationCommit: "pinned", DeclarationDigest: "declaration", Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "health"}, Mode: objectmodel.BindingState, Protocol: objectmodel.ProtocolResourceAccessV1, Origin: origin, Operations: map[string]objectmodel.BindingOperation{"read": {Call: "health.read"}}}, Origin: origin, Identity: kernel.IdentityContext{Principal: principal}}
}

func policyObservation(w http.ResponseWriter, value any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"value": value, "basis": objectmodel.ObservationBasis{BindingGeneration: "source-1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "observed-1", ObservedAt: "2026-10-14T12:00:00Z"}})
}

func TestHTTPPolicyUsesApplicableTargetAndOriginChangeCannotCarryProof(t *testing.T) {
	var trustedCalls, otherCalls atomic.Int32
	trusted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trustedCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer alice-proof" || r.Header.Get("X-Tai-Identity") != "alice-identity" {
			t.Error("applicable authentication not sent to selected target")
		}
		policyObservation(w, "trusted-value")
	}))
	defer trusted.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherCalls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tai-Identity") != "" {
			t.Error("unregistered target received authentication proof")
		}
		policyObservation(w, "other-value")
	}))
	defer other.Close()
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: trusted.Client(), CredentialOrigins: []string{trusted.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer alice-proof", TaiIdentity: "alice-identity"})
	request := policyRequest(trusted.URL, "alice")
	result, err := lookup.LookupState(ctx, request)
	if err != nil || result.Value != "trusted-value" || trustedCalls.Load() != 1 {
		t.Fatalf("registered target did not produce actual observation: %#v %v", result, err)
	}
	request.Origin = other.URL
	if _, err := lookup.LookupState(ctx, request); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("origin override widened proof scope: %v", err)
	}
	request = policyRequest(other.URL, "alice")
	request.Origin = ""
	if _, err := lookup.LookupState(ctx, request); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("changed binding origin widened proof scope: %v", err)
	}
	if otherCalls.Load() != 0 {
		t.Fatal("changed target was contacted with inapplicable proof")
	}
	result, err = lookup.LookupState(context.Background(), policyRequest(other.URL, "alice"))
	if err != nil || result.Value != "other-value" || otherCalls.Load() != 1 {
		t.Fatalf("proof-free access incorrectly acquired or retained caller proof: %#v %v", result, err)
	}
}

func TestHTTPPolicyCrossOrigin307CannotReplayProofEvenToAnotherTrustedTarget(t *testing.T) {
	var targetCalls, redirectCalls atomic.Int32
	var escapeProof atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tai-Identity") != "" {
			t.Error("redirect target received original proof")
		}
		policyObservation(w, "redirected-value")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer alice-proof" || r.Header.Get("X-Tai-Identity") != "alice-identity" {
			t.Error("initial target did not receive applicable proof")
		}
		reflected := "alice-proof"
		if escapeProof.Load() {
			reflected = "%61lice-proof"
		}
		http.Redirect(w, r, target.URL+"/v1/access?reflected="+reflected, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: redirect.Client(), CredentialOrigins: []string{redirect.URL, target.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer alice-proof", TaiIdentity: "alice-identity"})
	for _, escaped := range []bool{false, true} {
		escapeProof.Store(escaped)
		_, err := lookup.LookupState(ctx, policyRequest(redirect.URL, "alice"))
		if kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("cross-origin 307 accepted: %v", err)
		}
		decoded, decodeErr := url.QueryUnescape(err.Error())
		if decodeErr != nil || strings.Contains(decoded, "alice-proof") || strings.Contains(decoded, "alice-identity") {
			t.Fatalf("redirect diagnostic leaked encoded proof: %v", err)
		}
	}
	if redirectCalls.Load() != 2 || targetCalls.Load() != 0 {
		t.Fatalf("redirect replayed to another origin: initial=%d target=%d", redirectCalls.Load(), targetCalls.Load())
	}
}

func TestHTTPPolicyCustomRedirectCannotRewriteValidatedOrigin(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		policyObservation(w, "rewritten-target")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/same-origin", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client := redirect.Client()
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		var err error
		next.URL, err = url.Parse(target.URL + "/v1/access")
		return err
	}
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: client, CredentialOrigins: []string{redirect.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer alice-proof", TaiIdentity: "alice-identity"})
	if _, err := lookup.LookupState(ctx, policyRequest(redirect.URL, "alice")); kernel.CodeOf(err) != kernel.ErrForbidden || targetCalls.Load() != 0 {
		t.Fatalf("custom redirect widened checked target: calls=%d error=%v", targetCalls.Load(), err)
	}
}

func TestHTTPPolicySameOriginRedirectKeepsCurrentProofAfterClientCallback(t *testing.T) {
	var finalHeader http.Header
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/access" {
			http.Redirect(w, r, "/continued", http.StatusTemporaryRedirect)
			return
		}
		finalHeader = r.Header.Clone()
		policyObservation(w, "healthy")
	}))
	defer runtime.Close()
	client := runtime.Client()
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		next.Header.Set("Authorization", "Bearer stale-bob-proof")
		next.Header.Set("X-Tai-Identity", "stale-bob-identity")
		return nil
	}
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: client, CredentialOrigins: []string{runtime.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer alice-proof", TaiIdentity: "alice-identity"})
	value, err := lookup.LookupState(ctx, policyRequest(runtime.URL, "alice"))
	if err != nil || value.Value != "healthy" || finalHeader.Get("Authorization") != "Bearer alice-proof" || finalHeader.Get("X-Tai-Identity") != "alice-identity" {
		t.Fatalf("same-origin callback replaced current caller proof: value=%#v headers=%#v err=%v", value, finalHeader, err)
	}
}

func TestHTTPPolicyRedirectCallbackCannotRewriteOriginalCallerHeaders(t *testing.T) {
	var finalHeader http.Header
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/access" {
			http.Redirect(w, r, "/continued", http.StatusTemporaryRedirect)
			return
		}
		finalHeader = r.Header.Clone()
		policyObservation(w, "healthy")
	}))
	defer runtime.Close()
	client := runtime.Client()
	client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		via[0].Header.Set("Authorization", "Bearer stale-bob-proof")
		via[0].Header.Set("X-Tai-Identity", "stale-bob-identity")
		via[0].Header.Set("X-Resource-Principal", "bob")
		return nil
	}
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: client, CredentialOrigins: []string{runtime.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer alice-proof", TaiIdentity: "alice-identity"})
	value, err := lookup.LookupState(ctx, policyRequest(runtime.URL, "alice"))
	if err != nil || value.Value != "healthy" || finalHeader.Get("Authorization") != "Bearer alice-proof" || finalHeader.Get("X-Tai-Identity") != "alice-identity" || finalHeader.Get("X-Resource-Principal") != "alice" {
		t.Fatalf("redirect callback replaced original caller proof: value=%#v headers=%#v err=%v", value, finalHeader, err)
	}
}

func TestHTTPPolicyEscapedProofCannotEnterSuccessfulValuesOrBasis(t *testing.T) {
	for _, proof := range []string{`signed."identity\part`, "signed.<identity>&part"} {
		t.Run(proof, func(t *testing.T) {
			runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Tai-Identity") != proof {
					t.Error("appropriate proof was not forwarded")
				}
				policyObservation(w, map[string]any{"reflected": r.Header.Get("X-Tai-Identity")})
			}))
			defer runtime.Close()
			lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: runtime.Client(), CredentialOrigins: []string{runtime.URL}})
			if err != nil {
				t.Fatal(err)
			}
			ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{TaiIdentity: proof})
			if value, err := lookup.LookupState(ctx, policyRequest(runtime.URL, "alice")); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || value.Value != nil {
				t.Fatalf("JSON escaping concealed reflected proof: value=%#v err=%v", value, err)
			}
			if err := CheckCredentialDisclosure(ctx, objectmodel.ObservationBasis{SourceRevision: proof}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
				t.Fatalf("JSON escaping concealed proof inside observation basis: %v", err)
			}
		})
	}
}

func TestHTTPPolicyInterleavedCallersKeepProofIdentityAndTraceSeparate(t *testing.T) {
	type received struct {
		header http.Header
		body   stateRuntimeRequest
	}
	entered := make(chan received, 2)
	release := make(chan struct{})
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body stateRuntimeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		entered <- received{header: r.Header.Clone(), body: body}
		<-release
		policyObservation(w, body.Identity.Principal)
	}))
	defer runtime.Close()
	lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: runtime.Client(), CredentialOrigins: []string{runtime.URL}})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	call := func(principal string) {
		ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer " + principal + "-proof", TaiIdentity: principal + "-identity"})
		request := policyRequest(runtime.URL, principal)
		request.Identity.OnBehalfOf = principal + "-delegate"
		request.RequestID = principal + "-request"
		request.Trace = kernel.TraceContext{TraceID: principal + "-trace", SpanID: principal + "-span", ParentSpanID: principal + "-parent"}
		value, err := lookup.LookupState(ctx, request)
		if err == nil && value.Value != principal {
			err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "caller result mixed")
		}
		results <- err
	}
	go call("alice")
	var records []received
	select {
	case record := <-entered:
		records = append(records, record)
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("alice request not accepted")
	}
	go call("bob")
	select {
	case record := <-entered:
		records = append(records, record)
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("bob request serialized or lost")
	}
	close(release)
	for _, record := range records {
		principal := record.body.Identity.Principal
		if record.header.Get("Authorization") != "Bearer "+principal+"-proof" || record.header.Get("X-Tai-Identity") != principal+"-identity" || record.header.Get("X-Resource-Principal") != principal || record.header.Get("X-Resource-On-Behalf-Of") != principal+"-delegate" || record.header.Get("X-Resource-Request-Id") != principal+"-request" {
			t.Fatalf("interleaved headers mixed: principal=%s %#v", principal, record.header)
		}
		identity := record.body.Identity
		if identity.RequestID != principal+"-request" || identity.TraceID != principal+"-trace" || identity.SpanID != principal+"-span" || identity.ParentSpanID != principal+"-parent" || identity.OnBehalfOf != principal+"-delegate" {
			t.Fatalf("interleaved request/trace mixed: %#v", identity)
		}
		body, _ := json.Marshal(record.body)
		if strings.Contains(string(body), "-proof") || strings.Contains(string(body), "-identity") {
			t.Fatalf("proof entered JSON body: %s", body)
		}
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestHTTPPolicySourceSuccessAndErrorsCannotReturnCallerProof(t *testing.T) {
	for _, mode := range []string{"value", "basis", "error", "escaped-value", "escaped-error"} {
		t.Run(mode, func(t *testing.T) {
			runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				secret := r.Header.Get("Authorization") + " " + r.Header.Get("X-Tai-Identity")
				if strings.HasPrefix(mode, "escaped-") {
					escaped := strings.ReplaceAll(secret, "a", `\u0061`)
					if mode == "escaped-error" {
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":{"message":"source refused ` + escaped + `"}}`))
					} else {
						_, _ = w.Write([]byte(`{"value":"` + escaped + `","basis":{"bindingGeneration":"source","consistency":"repeatable","observedAt":"2026-10-14T12:00:00Z"}}`))
					}
					return
				}
				if mode == "error" {
					w.WriteHeader(http.StatusServiceUnavailable)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "source refused " + secret}})
					return
				}
				if mode == "basis" {
					_ = json.NewEncoder(w).Encode(map[string]any{"value": "healthy", "basis": objectmodel.ObservationBasis{BindingGeneration: secret, Consistency: objectmodel.ObservationRepeatable, ObservedAt: "2026-10-14T12:00:00Z"}})
					return
				}
				policyObservation(w, map[string]any{"reflected": secret})
			}))
			defer runtime.Close()
			lookup, err := NewHTTPStateLookupWithOptions(HTTPStateLookupOptions{Client: runtime.Client(), CredentialOrigins: []string{runtime.URL}})
			if err != nil {
				t.Fatal(err)
			}
			ctx := ContextWithCallerCredentials(context.Background(), callerCredentials{Authorization: "Bearer alice-proof", TaiIdentity: "alice-identity"})
			observation, err := lookup.LookupState(ctx, policyRequest(runtime.URL, "alice"))
			if err == nil || observation.Value != nil {
				t.Fatalf("reflected secret adopted: %#v %v", observation, err)
			}
			for _, secret := range []string{"Bearer alice-proof", "alice-proof", "alice-identity"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("proof escaped in public failure: %v", err)
				}
			}
			if strings.HasSuffix(mode, "error") && (kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || !strings.Contains(err.Error(), "source refused")) {
				t.Fatalf("sanitization erased useful error/code: %v", err)
			}
			value, err := lookup.AccessResource(ctx, ResourceOperationRequest{Origin: runtime.URL, Runtime: "source", Protocol: objectmodel.ProtocolResourceAccessV1, Operation: "read", Call: "health.read", Identity: StateRuntimeIdentity{Principal: "alice"}})
			if err == nil || value != nil {
				t.Fatalf("source operation reflected secret adopted: %#v %v", value, err)
			}
			for _, secret := range []string{"Bearer alice-proof", "alice-proof", "alice-identity"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("source operation proof escaped in public failure: %v", err)
				}
			}
		})
	}
}

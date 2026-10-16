package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"kc/identity"
	"kc/identity/identitytest"
	localauth "kc/identity/local"
	"kc/kernel"
)

func TestSnapshotIdentityRequiresExplicitLocalAndRejectsForgedDelegation(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/identity/v1/whoami", nil)
	request.Header.Set("X-Kc-As", "alice")
	for _, options := range []IdentityOptions{{}, {Mode: "local"}, {Mode: "taihu"}} {
		if _, err := AuthenticateRequest(request, options); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
			t.Fatalf("missing verifier implicitly accepted local identity: %#v %v", options, err)
		}
	}
	local := IdentityOptions{Authenticator: localauth.New()}
	if caller, err := AuthenticateRequest(request, local); err != nil || caller.Principal != "alice" {
		t.Fatalf("explicit fixture identity: %#v %v", caller, err)
	}
	request.Header.Set("X-Kc-On-Behalf-Of", "bob")
	if _, err := AuthenticateRequest(request, local); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("local proxy header became trusted delegation: %v", err)
	}
	request.Header.Del("X-Kc-On-Behalf-Of")
	request.Header.Set("Authorization", "Bearer proof")
	if _, err := AuthenticateRequest(request, local); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("local mode accepted transport credentials: %v", err)
	}
	verifier := &identitytest.Recorder{Mode: "verified", Caller: identity.HTTPIdentity{Principal: "alice", OnBehalfOf: "trusted-user"}}
	options := IdentityOptions{Authenticator: verifier}
	if _, err := AuthenticateRequest(request, options); kernel.CodeOf(err) != kernel.ErrForbidden || verifier.Calls != 0 {
		t.Fatalf("forged principal reached the binding verifier: %v %d", err, verifier.Calls)
	}
	request.Header.Del("X-Kc-As")
	request.Header.Set("X-Kc-On-Behalf-Of", "bob")
	if _, err := AuthenticateRequest(request, options); kernel.CodeOf(err) != kernel.ErrForbidden || verifier.Calls != 0 {
		t.Fatalf("forged delegation reached the binding verifier: %v %d", err, verifier.Calls)
	}
	request.Header.Del("X-Kc-On-Behalf-Of")
	if caller, err := AuthenticateRequest(request, options); err != nil || caller.OnBehalfOf != "trusted-user" {
		t.Fatalf("verified delegation was lost: %#v %v", caller, err)
	}
}

func TestSnapshotWhoAmIAndDiscoveryDoNotBorrowResourcesOrExposeProviderIdentity(t *testing.T) {
	verifier := &identitytest.Recorder{Mode: "taihu", Caller: identity.HTTPIdentity{Principal: "alice", Login: "alice", Provider: "taihu", Subject: "immutable-subject", Admin: true}}
	observed := 0
	handler := newHandler(Config{
		Identity: IdentityOptions{Authenticator: verifier, BrowserLogin: map[string]string{"provider": "external"}},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
			t.Fatal("identity transport tried to open application resources")
			return Resources{}, nil, nil
		},
		Observe: func(ctx context.Context, caller identity.HTTPIdentity, operation string) (context.Context, func(any, error)) {
			if caller.Principal != "alice" || operation != "whoami" {
				t.Fatalf("identity observation: %#v %s", caller, operation)
			}
			return ctx, func(_ any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				observed++
			}
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/identity/v1/auth", nil)
	w := httptest.NewRecorder()
	handler.identityAuth(w, r)
	var discovery map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &discovery); err != nil {
		t.Fatal(err)
	}
	if discovery["mode"] != "taihu" || discovery["localAssertion"] != false || !reflect.DeepEqual(discovery["accepts"], []any{"Authorization"}) || discovery["browserLogin"] == nil || verifier.Calls != 0 {
		t.Fatalf("unauthenticated pairing metadata changed: %#v calls=%d", discovery, verifier.Calls)
	}
	r = httptest.NewRequest(http.MethodGet, "/identity/v1/whoami", nil)
	r.Header.Set("Authorization", "Bearer proof")
	w = httptest.NewRecorder()
	handler.identityWhoAmI(w, r)
	var caller map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &caller); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || !reflect.DeepEqual(caller, map[string]any{"principal": "alice", "login": "alice", "onBehalfOf": ""}) || observed != 1 {
		t.Fatalf("whoami public representation: %d %#v observed=%d", w.Code, caller, observed)
	}
}

func TestSnapshotLocalDiscoveryAdvertisesSelfAssertion(t *testing.T) {
	handler := newHandler(Config{Identity: IdentityOptions{Authenticator: localauth.New()}})
	w := httptest.NewRecorder()
	handler.identityAuth(w, httptest.NewRequest(http.MethodGet, "/identity/v1/auth", nil))
	var discovery map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &discovery); err != nil {
		t.Fatal(err)
	}
	if discovery["mode"] != "local" || discovery["localAssertion"] != true || !reflect.DeepEqual(discovery["accepts"], []any{"X-Kc-As"}) {
		t.Fatalf("local pairing metadata: %#v", discovery)
	}
}

// The browser request check belongs to the service, not to one authenticator:
// any verifying pairing sees ambient cookie mutations rejected first.
func TestSnapshotBrowserCookieMutationsRejectBeforeVerifierSideEffects(t *testing.T) {
	verifier := &identitytest.Recorder{Mode: "verified", Caller: identity.HTTPIdentity{Principal: "alice"}}
	options := IdentityOptions{Authenticator: verifier}
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/grants", nil)
	request.Header.Set("Cookie", "internal_auth_session=proof")
	for _, headers := range []http.Header{
		{},
		{"X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"cross-site"}, "Content-Type": {"application/json"}},
		{"X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"same-origin"}, "Content-Type": {"text/plain"}},
	} {
		for name := range request.Header {
			if name != "Cookie" {
				delete(request.Header, name)
			}
		}
		for name, values := range headers {
			request.Header[name] = values
		}
		if _, err := AuthenticateRequest(request, options); kernel.CodeOf(err) != kernel.ErrForbidden || verifier.Calls != 0 {
			t.Fatalf("unsafe cookie mutation reached verifier: %v calls=%d", err, verifier.Calls)
		}
	}
}

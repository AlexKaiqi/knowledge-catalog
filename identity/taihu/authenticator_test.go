package taihu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/identity"
	"kc/kernel"
)

// TestTaihuIntrospectionURLIsSingleEndpoint guards against the double-append
// bug where the introspection URL got "/oauth2/introspect" concatenated twice
// (base construction + per-request), producing a 404. The server must receive
// exactly one "/oauth2/introspect" request path.
func TestTaihuIntrospectionURLIsSingleEndpoint(t *testing.T) {
	var receivedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active":   true,
			"sub":      "12345",
			"subject":  "12345",
			"username": "alice",
		})
	}))
	defer ts.Close()

	a, err := New("", ts.URL, "knowledge-catalog", "secret", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if a.introspectionURL != ts.URL+"/oauth2/introspect" {
		t.Fatalf("introspectionURL = %q, want %q", a.introspectionURL, ts.URL+"/oauth2/introspect")
	}

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer some-token")
	id, err := a.Authenticate(context.Background(), hdr)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if receivedPath != "/oauth2/introspect" {
		t.Fatalf("introspection request path = %q, want exactly /oauth2/introspect (no double append)", receivedPath)
	}
	if id.Principal != "alice" {
		t.Fatalf("principal = %q, want alice", id.Principal)
	}
	if id.Login != "alice" {
		t.Fatalf("login = %q, want alice", id.Login)
	}
	if id.Subject != "12345" {
		t.Fatalf("subject = %q, want 12345", id.Subject)
	}
}

func TestTaihuRejectsUnverifiedBearerWithoutIntrospectionOrGateway(t *testing.T) {
	a, err := New("", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer unverified")
	if _, err := a.Authenticate(context.Background(), hdr); err == nil || kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("unverified Bearer must be UNAUTHENTICATED: %v", err)
	}
}

func TestTaihuIntrospectionMapsUserAgentAndServicePrincipals(t *testing.T) {
	cases := []struct {
		name      string
		body      map[string]any
		principal string
		onBehalf  string
		login     string
		err       bool
	}{
		{
			name:      "user",
			body:      map[string]any{"active": true, "sub": "12345", "client_id": "knowledge-catalog", "username": "alice"},
			principal: "alice",
			login:     "alice",
		},
		{
			name: "agent",
			body: map[string]any{
				"active": true, "sub": "12345", "client_id": "knowledge-catalog", "username": "alice",
				"act": map[string]any{"sub": "dsh"},
			},
			principal: "agent:dsh",
			onBehalf:  "alice",
			login:     "alice",
		},
		{
			name:      "service",
			body:      map[string]any{"active": true, "client_id": "batch-sync"},
			principal: "service:batch-sync",
		},
		{
			name: "user missing username",
			body: map[string]any{"active": true, "sub": "12345", "client_id": "knowledge-catalog"},
			err:  true,
		},
		{
			name: "agent missing username",
			body: map[string]any{
				"active": true, "sub": "12345", "client_id": "knowledge-catalog",
				"act": map[string]any{"sub": "dsh"},
			},
			err: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			t.Cleanup(ts.Close)
			a, err := New("", ts.URL, "knowledge-catalog", "secret", ts.Client())
			if err != nil {
				t.Fatal(err)
			}
			hdr := http.Header{}
			hdr.Set("Authorization", "Bearer token")
			id, err := a.Authenticate(context.Background(), hdr)
			if tc.err {
				if err == nil || kernel.CodeOf(err) != kernel.ErrUnauthenticated {
					t.Fatalf("missing username must be UNAUTHENTICATED: %#v %v", id, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id.Principal != tc.principal || id.OnBehalfOf != tc.onBehalf || id.Login != tc.login {
				t.Fatalf("identity = %#v, want principal %q onBehalfOf %q login %q", id, tc.principal, tc.onBehalf, tc.login)
			}
		})
	}
}

// TestTaihuIntrospectionBaseHandlesOAuth2Suffix verifies that passing a base
// URL that already ends with /oauth2 does not produce a doubled path.
func TestTaihuIntrospectionBaseHandlesOAuth2Suffix(t *testing.T) {
	a, err := New("", "http://iam.example/oauth2", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.introspectionURL != "http://iam.example/oauth2/introspect" {
		t.Fatalf("introspectionURL = %q, want http://iam.example/oauth2/introspect", a.introspectionURL)
	}
}

// TestAuthHmacSecretReadsFlagOrEnv verifies gateway HMAC comes from
// --auth-hmac-secret or KC_TAIHU_HMAC_SECRET. Values are test fixtures,
// not production secrets.
func TestAuthHmacSecretReadsFlagOrEnv(t *testing.T) {
	t.Setenv("KC_TAIHU_HMAC_SECRET", "")
	fixture := hex.EncodeToString([]byte("kc-test-hmac"))

	auth, err := identity.Resolve("taihu", settings{
		"auth-hmac-secret": fixture,
	})
	if err != nil {
		t.Fatalf("valid --auth-hmac-secret must be accepted: %v", err)
	}
	ta, ok := auth.(*Authenticator)
	if !ok {
		t.Fatalf("expected *Authenticator, got %T", auth)
	}
	if string(ta.hmacSecret) != "kc-test-hmac" {
		t.Fatalf("hmac secret = %q, want kc-test-hmac", ta.hmacSecret)
	}

	if _, err := identity.Resolve("taihu", settings{
		"auth-hmac-secret": "not-hex!!",
	}); err == nil {
		t.Fatal("invalid hex --auth-hmac-secret must fail")
	} else if !strings.Contains(err.Error(), "hex-encoded") {
		t.Fatalf("expected hex decode error, got: %v", err)
	}

	t.Setenv("KC_TAIHU_HMAC_SECRET", fixture)
	fromEnv, err := identity.Resolve("taihu", settings{})
	if err != nil {
		t.Fatalf("KC_TAIHU_HMAC_SECRET must be accepted: %v", err)
	}
	ta, ok = fromEnv.(*Authenticator)
	if !ok {
		t.Fatalf("expected *Authenticator, got %T", fromEnv)
	}
	if string(ta.hmacSecret) != "kc-test-hmac" {
		t.Fatalf("env hmac secret = %q, want kc-test-hmac", ta.hmacSecret)
	}

	t.Setenv("KC_TAIHU_HMAC_SECRET", hex.EncodeToString([]byte("from-env")))
	fromFlag, err := identity.Resolve("taihu", settings{
		"auth-hmac-secret": fixture,
	})
	if err != nil {
		t.Fatal(err)
	}
	ta, ok = fromFlag.(*Authenticator)
	if !ok {
		t.Fatalf("expected *Authenticator, got %T", fromFlag)
	}
	if string(ta.hmacSecret) != "kc-test-hmac" {
		t.Fatalf("flag must override env: got %q", ta.hmacSecret)
	}
}

func TestTaihuGatewayIdentityUsesUsernameNotStaffID(t *testing.T) {
	secret := []byte("fixture-hmac-secret")
	a, err := New(hex.EncodeToString(secret), "https://identity.test", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	hdr := http.Header{}
	hdr.Set("X-Tai-Identity", signedTaihuIdentity(secret, `{"staff_id":"12345","user_name":"alice"}`))
	id, err := a.Authenticate(context.Background(), hdr)
	if err != nil {
		t.Fatal(err)
	}
	if id.Principal != "alice" || id.Login != "alice" || id.Subject != "12345" || id.User == nil {
		t.Fatalf("gateway identity = %#v, want principal alice login alice subject 12345 and verified user", id)
	}

	hdr.Set("X-Tai-Identity", signedTaihuIdentity(secret, `{"staff_id":"12345","username":"bob"}`))
	id, err = a.Authenticate(context.Background(), hdr)
	if err != nil {
		t.Fatal(err)
	}
	if id.Principal != "bob" {
		t.Fatalf("username alias principal = %q, want bob", id.Principal)
	}

	hdr.Set("X-Tai-Identity", signedTaihuIdentity(secret, `{"staff_id":"12345"}`))
	if _, err := a.Authenticate(context.Background(), hdr); err == nil || kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("gateway identity without user_name must be UNAUTHENTICATED: %v", err)
	}

	hdr = http.Header{}
	hdr.Set("X-Tai-User", "alice")
	if _, err := a.Authenticate(context.Background(), hdr); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("bare x-tai-user must not establish a binding: %v", err)
	}
}

func signedTaihuIdentity(secret []byte, body string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString([]byte(body)) + "." + hex.EncodeToString(mac.Sum(nil))
}

type settings map[string]string

func (s settings) String(name string) string    { return s[name] }
func (s settings) Strings(name string) []string { return nil }

func TestTaihuGatewayRejectsUnsignedOrUnissuedIdentity(t *testing.T) {
	authenticator, err := New("6b632d746573742d686d6163", "", "kc", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, headers := range []http.Header{
		{"X-Tai-User": []string{"kaiqidong"}},
		{"X-Tai-Identity": []string{`{"staff_id":"42","user_name":"kaiqidong"}`}},
	} {
		if id, err := authenticator.Authenticate(context.Background(), headers); err == nil {
			t.Fatalf("unsigned gateway identity must not reserve a username: %#v", id)
		}
	}
	headers := http.Header{}
	headers.Set("X-Tai-Identity", signedTaihuIdentity([]byte("kc-test-hmac"), `{"staff_id":"42","user_name":"kaiqidong"}`))
	if got, err := authenticator.Authenticate(context.Background(), headers); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("signed identity without declared issuer occupied a username: %#v %v", got, err)
	}
}

func TestTaihuFactoryDeclaresLoginApplicationAndServicePrincipal(t *testing.T) {
	t.Setenv("KC_SERVICE_CLIENT_SECRET", "")
	bare, err := identity.Resolve("taihu", settings{"auth-url": "https://iam.example/oauth2/"})
	if err != nil {
		t.Fatal(err)
	}
	if identity.BrokerOf(bare) != nil || identity.ServicePrincipalOf(bare) != "" {
		t.Fatal("a deployment without a login application must not advertise a broker or service principal")
	}
	noSecret, err := identity.Resolve("taihu", settings{"auth-url": "https://iam.example", "service-client-id": "kc-app"})
	if err != nil {
		t.Fatal(err)
	}
	if identity.BrokerOf(noSecret) != nil || identity.ServicePrincipalOf(noSecret) != "taihu:service:kc-app" {
		t.Fatalf("client id without secret: broker %v principal %q", identity.BrokerOf(noSecret), identity.ServicePrincipalOf(noSecret))
	}
	full, err := identity.Resolve("taihu", settings{"auth-url": "https://iam.example/oauth2", "service-client-id": "kc-app", "service-client-secret": "s", "service-principal": "service:kc"})
	if err != nil {
		t.Fatal(err)
	}
	broker := identity.BrokerOf(identity.Observe(full, func(ctx context.Context, _ string) (context.Context, func(error)) { return ctx, func(error) {} }))
	if broker == nil || identity.ServicePrincipalOf(full) != "service:kc" {
		t.Fatal("configured application must be reachable through decorators")
	}
	if login := broker.BrowserLogin(); login.OAuth2Base != "https://iam.example" || login.ClientID != "kc-app" || login.Resource != "kc-app" || login.Scope != "openid profile" {
		t.Fatalf("browser login = %#v", login)
	}
	if _, err := identity.Resolve("taihu", settings{"auth-url": "iam.example"}); err == nil || !strings.Contains(err.Error(), "--auth taihu: --auth-url must be a Taihu http(s) origin") {
		t.Fatalf("non-HTTP issuer: %v", err)
	}
}

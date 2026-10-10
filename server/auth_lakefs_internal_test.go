package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
)

func lakeFSProof(cookie string) http.Header {
	return http.Header{"Cookie": {cookie}, "X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"same-origin"}}
}

func TestLakeFSCallerProofUsesOnlyTrustedVerifier(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/api/v1/user" || r.Method != http.MethodGet {
			t.Errorf("unexpected verification target: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Cookie") != "internal_auth_session=caller-session" || r.Header.Get("Authorization") != "" {
			t.Errorf("upstream proof was not filtered: %v", r.Header)
		}
		if r.Header.Get("X-Kc-As") != "" || r.Header.Get("X-Kc-Console") != "" {
			t.Error("caller metadata was forwarded to provider")
		}
		_, _ = io.WriteString(w, `{"user":{"id":"console-validation","creation_date":1700000000,"is_admin":true}}`)
	}))
	defer provider.Close()
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(provider.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "server-private", Value: "must-not-forward"}})
	client := provider.Client()
	client.Jar = jar
	auth, err := NewLakeFSAuthenticator(provider.URL+"/prefix/", client)
	if err != nil {
		t.Fatal(err)
	}
	headers := lakeFSProof("unrelated=discard; internal_auth_session=caller-session")
	headers.Set("X-Kc-As", "platform-admin")
	id, err := auth.Authenticate(context.Background(), headers)
	if err != nil || id.Principal != "console-validation" || id.Login != id.Principal || id.Admin || id.User == nil {
		t.Fatalf("verified caller or privilege isolation: %#v %v", id, err)
	}
	if id.User.Provider != "lakefs" || id.User.Issuer != provider.URL+"/prefix" || id.User.Subject == "" {
		t.Fatalf("missing binding coordinates: %#v", id.User)
	}
	if client.Jar != jar {
		t.Fatal("constructor mutated caller-owned HTTP client")
	}
}

func TestLakeFSCredentialAmbiguityAndCookieCSRF(t *testing.T) {
	called := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_, _ = io.WriteString(w, `{"user":{"id":"console-validation","creation_date":1700000000}}`)
	}))
	defer provider.Close()
	auth, err := NewLakeFSAuthenticator(provider.URL, provider.Client())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		headers http.Header
		code    kernel.ErrorCode
	}{
		{"missing", http.Header{}, kernel.ErrUnauthenticated},
		{"unknown-cookie", lakeFSProof("arbitrary=proof"), kernel.ErrUnauthenticated},
		{"mixed", http.Header{"Authorization": {"Bearer token"}, "Cookie": {"other=proof"}}, kernel.ErrUnauthenticated},
		{"duplicate-auth", http.Header{"Authorization": {"Bearer one", "Bearer two"}}, kernel.ErrUnauthenticated},
		{"duplicate-cookie", lakeFSProof("internal_auth_session=a; internal_auth_session=b"), kernel.ErrUnauthenticated},
		{"multiple-session-proofs", lakeFSProof("internal_auth_session=a; oidc_auth_session=b"), kernel.ErrUnauthenticated},
		{"empty-cookie", lakeFSProof("internal_auth_session="), kernel.ErrUnauthenticated},
		{"unknown-scheme", http.Header{"Authorization": {"Token token"}}, kernel.ErrUnauthenticated},
		{"missing-marker", http.Header{"Cookie": {"internal_auth_session=proof"}}, kernel.ErrForbidden},
		{"duplicate-marker", http.Header{"Cookie": {"internal_auth_session=proof"}, "X-Kc-Console": {"1", "1"}}, kernel.ErrForbidden},
		{"duplicate-fetch-metadata", http.Header{"Cookie": {"internal_auth_session=proof"}, "X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"same-origin", "cross-site"}}, kernel.ErrForbidden},
		{"cross-site", http.Header{"Cookie": {"internal_auth_session=proof"}, "X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"cross-site"}}, kernel.ErrForbidden},
		{"same-site", http.Header{"Cookie": {"internal_auth_session=proof"}, "X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"same-site"}}, kernel.ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := auth.Authenticate(context.Background(), tc.headers); kernel.CodeOf(err) != tc.code {
				t.Fatalf("invalid proof: %v", err)
			}
		})
	}
	if called != 0 {
		t.Fatal("invalid proof reached trusted verifier")
	}
	for _, h := range []http.Header{{"Authorization": {"Bearer caller-token"}}, {"Authorization": {"Basic dXNlcjpwYXNz"}}, lakeFSProof("oidc_auth_session=proof"), lakeFSProof("saml_auth_session=proof"), {"Cookie": {"internal_auth_session=proof"}, "X-Kc-Console": {"1"}}, {"Cookie": {"internal_auth_session=proof"}, "X-Kc-Console": {"1"}, "Sec-Fetch-Site": {"none"}}} {
		if _, err := auth.Authenticate(context.Background(), h); err != nil {
			t.Fatalf("supported proof rejected: %v", err)
		}
	}
}

func TestLakeFSVerifierFailClosedAndNoRedirect(t *testing.T) {
	redirectCalls := 0
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalls++
		_, _ = io.WriteString(w, `{"user":{"id":"console-validation","creation_date":1700000000}}`)
	}))
	defer redirect.Close()
	status, body := http.StatusOK, ""
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", redirect.URL)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	defer provider.Close()
	auth, err := NewLakeFSAuthenticator(provider.URL, provider.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{}`, `{"user":{"id":"console-validation"}}`, `{"user":{"id":"console-validation","creation_date":0}}`, `{"user":{"id":"console-validation","creation_date":-1}}`, `{"user":{"id":"Wrong Name","creation_date":1700000000}}`, `{"user":{"id":"console-validation","creation_date":1.5}}`, `{"user":{"id":"console-validation","creation_date":1700000000}} {}`, strings.Repeat(" ", (1<<20)+1)} {
		body = invalid
		if id, err := auth.Authenticate(context.Background(), http.Header{"Authorization": {"Bearer proof"}}); err == nil || id.User != nil {
			t.Fatalf("invalid upstream user established an identity: %#v %v", id, err)
		}
	}
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNoContent, http.StatusCreated, http.StatusTemporaryRedirect, http.StatusInternalServerError} {
		status, body = code, `{"user":{"id":"console-validation","creation_date":1700000000},"secret":"upstream-private"}`
		_, err := auth.Authenticate(context.Background(), http.Header{"Authorization": {"Bearer caller-private"}})
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("upstream failure leaked proof or was accepted: %v", err)
		}
	}
	if redirectCalls != 0 {
		t.Fatal("trusted verifier redirect was followed")
	}
	for _, origin := range []string{"", "file:///tmp/user", "https://user:secret@example.com", "https://example.com?x=1", "https://example.com#fragment", "https://example.com/api/v1"} {
		if _, err := NewLakeFSAuthenticator(origin, nil); err == nil {
			t.Fatalf("invalid verifier origin accepted: %q", origin)
		}
	}
}

func TestLakeFSBindingRejectsRecreatedAccountAndConsoleMutation(t *testing.T) {
	state := t.TempDir()
	created := int64(1700000000)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": "console-validation", "creation_date": created}})
	}))
	defer provider.Close()
	auth, err := NewLakeFSAuthenticator(provider.URL, provider.Client())
	if err != nil {
		t.Fatal(err)
	}
	newServer := func() *httptest.Server {
		h := HTTPHandlerWithOptions(state, HTTPServerOptions{Authenticator: auth})
		t.Cleanup(func() { _ = h.(interface{ Close() error }).Close() })
		return httptest.NewServer(h)
	}
	srv := newServer()
	headers := lakeFSProof("internal_auth_session=caller-session")
	if status, body := pairingGET(t, srv, "/identity/v1/whoami", headers); status != http.StatusOK || body["principal"] != "console-validation" {
		t.Fatalf("verified native session: %d %#v", status, body)
	}
	originRequest, _ := http.NewRequest(http.MethodGet, srv.URL+"/identity/v1/whoami", nil)
	originRequest.Header = headers.Clone()
	originRequest.Header.Set("Origin", "https://untrusted.example")
	originResponse, err := srv.Client().Do(originRequest)
	if err != nil {
		t.Fatal(err)
	}
	originResponse.Body.Close()
	if originResponse.Header.Get("Access-Control-Allow-Origin") != "" || originResponse.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("native session verification enabled credentialed cross-origin access")
	}
	_, discovery := pairingGET(t, srv, "/identity/v1/auth", nil)
	if discovery["mode"] != "lakefs" || !reflect.DeepEqual(discovery["accepts"], []any{"Cookie", "Authorization"}) || discovery["localAssertion"] != false {
		t.Fatalf("native session discovery: %#v", discovery)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
			req := httptest.NewRequest(method, "/", strings.NewReader(`{}`))
			req.Header = headers.Clone()
			req.Header.Set("Content-Type", contentType)
			w := httptest.NewRecorder()
			if _, ok := authenticateHTTPRequest(w, req, HTTPServerOptions{Authenticator: auth}); ok || w.Code != http.StatusForbidden {
				t.Fatalf("cookie mutation with content type %q accepted: %d", contentType, w.Code)
			}
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header = headers.Clone()
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if _, ok := authenticateHTTPRequest(httptest.NewRecorder(), req, HTTPServerOptions{Authenticator: auth}); !ok {
		t.Fatal("JSON console mutation rejected")
	}
	for _, authorization := range []string{"Bearer caller-token", "Basic dXNlcjpwYXNz"} {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		req.Header.Set("Authorization", authorization)
		if _, ok := authenticateHTTPRequest(httptest.NewRecorder(), req, HTTPServerOptions{Authenticator: auth}); !ok {
			t.Fatal("nonbrowser credential wrongly required console headers or JSON body")
		}
	}
	srv.Close()
	created++
	srv = newServer()
	defer srv.Close()
	if status, _ := pairingGET(t, srv, "/identity/v1/whoami", headers); status != http.StatusForbidden {
		t.Fatalf("recreated native user inherited reserved username after restart: %d", status)
	}
}

func TestLakeFSIncompatibleVerifiedAccountDoesNotBecomeLoginFailure(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"user":{"id":"user@example.com","creation_date":1700000000}}`)
	}))
	defer provider.Close()
	auth, err := NewLakeFSAuthenticator(provider.URL, provider.Client())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/identity/v1/whoami", nil)
	request.Header = lakeFSProof("internal_auth_session=valid-native-session")
	w := httptest.NewRecorder()
	if _, ok := authenticateHTTPRequest(w, request, HTTPServerOptions{Authenticator: auth}); ok || w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "explicit identity association") {
		t.Fatalf("valid native identity caused relogin or implicit renaming: %d %s", w.Code, w.Body.String())
	}
}

func TestLakeFSHTTPServerOptionsRequireFixedOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://user:secret@example.com", "https://example.com/api/v1", "https://example.com?secret=value"} {
		if _, err := HTTPServerOptionsFromFlags(map[string]FlagValue{"auth": "lakefs", "auth-url": origin}); err == nil {
			t.Fatal("invalid or missing fixed verifier origin accepted")
		}
	}
	options, err := HTTPServerOptionsFromFlags(map[string]FlagValue{"auth": "lakefs", "auth-url": "https://lakefs.example/prefix"})
	if err != nil || options.Authenticator == nil || options.AuthModeLabel() != "lakefs" || options.localAssertion() {
		t.Fatalf("native verifier mode wiring: %#v %v", options, err)
	}
}

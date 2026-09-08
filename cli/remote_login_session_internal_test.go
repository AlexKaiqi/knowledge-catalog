package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kc/kernel"
)

func isolateLoginConfig(t *testing.T) {
	t.Helper()
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_SERVER_URL", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_AUTH_TOKEN", "")
}

func loginTestServer(t *testing.T, principal string, request func(*http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if request != nil {
			request(r)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/identity/v1/auth":
			_, _ = io.WriteString(w, `{"mode":"gitea","localAssertion":false}`)
		case "/identity/v1/whoami":
			_ = json.NewEncoder(w).Encode(map[string]string{"principal": principal})
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestLoginSavedCredentialNeverCrossesServer(t *testing.T) {
	isolateLoginConfig(t)
	serverA := loginTestServer(t, "alice", nil)
	var leaked string
	serverB := loginTestServer(t, "bob", func(r *http.Request) { leaked = r.Header.Get("Authorization") })
	if result := Run([]string{"login", "--server", serverA.URL, "--mode", "token", "--token", "alice-secret"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	result := Run([]string{"whoami", "--server", serverB.URL})
	if leaked != "" {
		t.Fatalf("LOGIN-SERVER-ISOLATION: credential sent to unrelated server: %q", leaked)
	}
	if result.Status == 0 {
		t.Fatalf("unrelated server must require its own login: %s", result.Stdout)
	}
}

func TestLoginPersistsDefaultServerAndIndependentSessions(t *testing.T) {
	isolateLoginConfig(t)
	serverA := loginTestServer(t, "alice", nil)
	serverB := loginTestServer(t, "bob", nil)
	for _, entry := range []struct{ server, token string }{{serverA.URL, "alice-token"}, {serverB.URL, "bob-token"}} {
		if result := Run([]string{"login", "--server", entry.server, "--mode", "token", "--token", entry.token}); result.Status != 0 {
			t.Fatal(result.Stdout)
		}
	}
	if got := remoteServerURL(nil); got != serverB.URL {
		t.Fatalf("LOGIN-DEFAULT-ENTRY: default server = %q, want %q", got, serverB.URL)
	}
	if result := Run([]string{"whoami"}); result.Status != 0 || !strings.Contains(result.Stdout, `"bob"`) {
		t.Fatalf("default server login continuity: %s", result.Stdout)
	}
	if result := Run([]string{"logout", "--server", serverB.URL}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if result := Run([]string{"whoami", "--server", serverA.URL}); result.Status != 0 || !strings.Contains(result.Stdout, `"alice"`) {
		t.Fatalf("logout of B lost A session: %s", result.Stdout)
	}
}

func TestTaihuLoginRequiresVerifiedPersistedIdentity(t *testing.T) {
	for _, failure := range []string{"whoami", "persistence"} {
		t.Run(failure, func(t *testing.T) {
			isolateLoginConfig(t)
			t.Setenv("KC_SERVICE_CLIENT_SECRET", "fixture-secret")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity/v1/token" {
					_, _ = io.WriteString(w, `{"accessToken":"token","expiresIn":3600}`)
					return
				}
				if failure == "whoami" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"error":{"code":"UNAUTHENTICATED","message":"rejected"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"principal":"alice"}`)
			}))
			t.Cleanup(server.Close)
			if failure == "persistence" {
				blocked := filepath.Join(t.TempDir(), "blocked")
				if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("KC_CONFIG_DIR", blocked)
			}
			result, err := exchangeTaihuCode(&invocation{Context: context.Background()}, taihuPendingAuth{OAuth2Base: server.URL, Server: server.URL, ClientID: "fixture", CodeVerifier: "verifier"}, "code", "http://localhost/callback")
			if err == nil {
				t.Fatalf("LOGIN-VERIFIED-DURABLE: %s failure returned success: %#v", failure, result)
			}
		})
	}
}

func TestSavedLoginRefreshesWithoutClientSecretAndKeepsServer(t *testing.T) {
	isolateLoginConfig(t)
	t.Setenv("KC_SERVICE_CLIENT_SECRET", "must-not-leave-client")
	var refreshCalls, whoamiCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/v1/token":
			refreshCalls++
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), "must-not-leave-client") || r.Header.Get("Authorization") != "" {
				t.Error("client disclosed application secret or old Authorization")
			}
			var request map[string]string
			_ = json.Unmarshal(raw, &request)
			if request["grantType"] != "refresh_token" || request["refreshToken"] != "refresh-old" {
				t.Errorf("wrong refresh request: %v", request)
			}
			_, _ = io.WriteString(w, `{"accessToken":"fresh","refreshToken":"refresh-new","expiresIn":3600}`)
		case "/identity/v1/whoami":
			whoamiCalls++
			if r.Header.Get("Authorization") != "Bearer fresh" || r.Header.Get("X-Kc-As") != "" {
				t.Errorf("wrong refreshed request headers")
			}
			_, _ = io.WriteString(w, `{"principal":"alice"}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	if err := persistTokenLogin(taihuSession{Server: server.URL, Principal: "alice", AccessToken: "old", RefreshToken: "refresh-old", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if result := Run([]string{"whoami"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if result := Run([]string{"whoami"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if refreshCalls != 1 || whoamiCalls != 3 {
		t.Fatalf("refresh calls=%d whoami=%d", refreshCalls, whoamiCalls)
	}
	session, ok, err := readServerTokenSession(server.URL)
	if err != nil || !ok || session.AccessToken != "fresh" || session.RefreshToken != "refresh-new" {
		t.Fatalf("refreshed login was not saved: ok=%v error=%v", ok, err)
	}
}

func TestExpiredSavedLoginNeverFallsBackToLocal(t *testing.T) {
	isolateLoginConfig(t)
	server := loginTestServer(t, "unexpected", func(*http.Request) { t.Error("expired token made a request") })
	if err := persistTokenLogin(taihuSession{Server: server.URL, Principal: "alice", AccessToken: "expired", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KC_AS", "forged-local")
	result := Run([]string{"whoami"})
	if result.Status == 0 || !strings.Contains(result.Stdout, "UNAUTHENTICATED") || !strings.Contains(result.Stdout, "kc login") {
		t.Fatalf("expired login must request reauthentication: %s", result.Stdout)
	}
}

func TestBrowserCodeCompletesThroughBrokerWithoutClientSecret(t *testing.T) {
	isolateLoginConfig(t)
	t.Setenv("KC_SERVICE_CLIENT_SECRET", "")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, secret, _ := r.BasicAuth()
		if secret != "server-secret" {
			t.Error("server did not provide its application secret")
		}
		_, _ = io.WriteString(w, `{"access_token":"verified-user-token","refresh_token":"refresh","expires_in":3600}`)
	}))
	t.Cleanup(upstream.Close)
	server := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{Authenticator: pairingAuthenticator{principal: "kaiqidong"}, ServiceIdentity: &ServiceIdentity{ClientID: "kc-app", ClientSecret: "server-secret", OAuth2Base: upstream.URL}}))
	t.Cleanup(server.Close)
	result, err := exchangeTaihuCode(&invocation{Context: context.Background()}, taihuPendingAuth{Server: server.URL, CodeVerifier: strings.Repeat("a", 43)}, "one-time-code", "http://localhost/callback")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "verified-user-token") || strings.Contains(string(raw), "server-secret") {
		t.Fatal("login output disclosed a secret")
	}
	if result := Run([]string{"whoami"}); result.Status != 0 || !strings.Contains(result.Stdout, `"kaiqidong"`) {
		t.Fatalf("verified browser login did not continue: %s", result.Stdout)
	}
}

func TestTaihuSessionPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-taihu.json")

	// Not present -> not ok.
	if _, ok := loadTaihuSession(path); ok {
		t.Fatal("load of missing session must report ok=false")
	}

	sess := taihuSession{
		Server:       "http://localhost:7380",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := persistTaihuSession(path, sess); err != nil {
		t.Fatal(err)
	}
	got, ok := loadTaihuSession(path)
	if !ok {
		t.Fatal("load of persisted session must be ok")
	}
	if got.Server != sess.Server || got.AccessToken != sess.AccessToken || got.RefreshToken != sess.RefreshToken {
		t.Fatalf("round-trip mismatch: %#v", got)
	}

	// Expired session -> not ok.
	expired := taihuSession{Server: sess.Server, AccessToken: "x", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := persistTaihuSession(path, expired); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadTaihuSession(path); ok {
		t.Fatal("expired session must be treated as absent")
	}

	// File permissions are restrictive (0600).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("session file must be 0600, got %o", perm)
	}
}

func TestLocalSessionPersistRoundTrip(t *testing.T) {
	isolateLoginConfig(t)
	t.Setenv("HOME", t.TempDir())
	if _, ok := loadLocalSession("http://kc-server:7380"); ok {
		t.Fatal("load of missing local session must report ok=false")
	}
	if err := persistLocalSession(localSession{Server: "http://kc-server:7380", Principal: "agent:dsh"}); err != nil {
		t.Fatal(err)
	}
	got, ok := loadLocalSession("http://kc-server:7380/")
	if !ok {
		t.Fatal("load of persisted local session must be ok")
	}
	if got.Principal != "agent:dsh" {
		t.Fatalf("principal = %q", got.Principal)
	}
	if _, ok := loadLocalSession("http://other:9"); ok {
		t.Fatal("local session must not apply to a different server")
	}
	info, err := os.Stat(localSessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("session file must be 0600, got %o", perm)
	}
}

func TestTaihuTokenExchangeRequiresClientSecretAndSurfacesError(t *testing.T) {
	pending := taihuPendingAuth{
		ClientID:     "knowledge-catalog",
		CodeVerifier: "verifier",
		OAuth2Base:   "http://example.invalid",
	}
	if _, err := exchangeTaihuAccessToken(pending, "code", "http://127.0.0.1:7382", "", nil); err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("missing secret: %v", err)
	}

	var sawSecret bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		user, pass, ok := r.BasicAuth()
		if r.Form.Get("client_secret") == "app-secret" || (ok && user == "knowledge-catalog" && pass == "app-secret") {
			sawSecret = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "live-token", "expires_in": 60})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"Taihu app authentication failed"}`)
	}))
	t.Cleanup(ts.Close)
	pending.OAuth2Base = ts.URL

	if _, err := exchangeTaihuAccessToken(pending, "code", "http://127.0.0.1:7382", "wrong", ts.Client()); err == nil || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("wrong secret must surface Taihu error, got %v", err)
	}
	got, err := exchangeTaihuAccessToken(pending, "code", "http://127.0.0.1:7382", "app-secret", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !sawSecret || got.AccessToken != "live-token" {
		t.Fatalf("token %#v sawSecret=%v", got, sawSecret)
	}
}

func TestEmbeddedExplicitHomeIgnoresClientDefaultsWithoutChangingProductBoundary(t *testing.T) {
	isolateLoginConfig(t)
	home := t.TempDir()
	if result := RunEmbeddedForTest([]string{"--home", home, "local", "init", "--catalog", "kr://fixture/catalog"}, nil); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		t.Error("explicit embedded Home unexpectedly contacted a default Server")
		w.WriteHeader(500)
	}))
	defer server.Close()
	if err := persistLocalSession(localSession{Server: server.URL, Principal: "agent:remote"}); err != nil {
		t.Fatal(err)
	}
	for _, defaultSource := range []string{"saved", "environment"} {
		t.Run(defaultSource, func(t *testing.T) {
			if defaultSource == "environment" {
				t.Setenv("KC_SERVER_URL", server.URL)
			}
			args := []string{"--home", home, "--as", "agent:fixture", "whoami"}
			embedded := RunEmbeddedForTest(args, nil)
			if embedded.Status != 0 || !strings.Contains(embedded.Stdout, "agent:fixture") {
				t.Fatalf("explicit fixture Home did not preserve embedded identity: %s", embedded.Stdout)
			}
			product := Run(args)
			if product.Status == 0 || !strings.Contains(product.Stdout, "mutually exclusive") {
				t.Fatalf("public CLI accepted embedded Home: %s", product.Stdout)
			}
			explicitBoth := RunEmbeddedForTest(append(args, "--server", server.URL), nil)
			if explicitBoth.Status == 0 || !strings.Contains(explicitBoth.Stdout, "mutually exclusive") {
				t.Fatalf("explicit server + home was accepted: %s", explicitBoth.Stdout)
			}
		})
	}
}

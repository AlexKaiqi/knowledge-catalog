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

	kcclient "kc/client"
	kcidentity "kc/identity"
	"kc/identity/identitytest"
	"kc/identity/taihu"
	"kc/internal/clientconfig"
	"kc/internal/jsonfile"
	inv "kc/invocation"
	"kc/kernel"
	serv "kc/server"
)

// testTaihuAuthenticator reports the taihu mode name because the identity-token
// broker is only served by an OAuth2 identity provider.
func testTaihuAuthenticator(principal string) identitytest.Authenticator {
	return identitytest.Static("taihu", kcidentity.HTTPIdentity{Principal: principal, Admin: principal == "kaiqidong"})
}

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
			_, _ = io.WriteString(w, `{"mode":"taihu","localAssertion":false}`)
		case "/identity/v1/whoami":
			_ = json.NewEncoder(w).Encode(map[string]string{"principal": principal})
		case "/catalog/v1/catalogs":
			_ = json.NewEncoder(w).Encode(map[string]any{"catalogs": []map[string]string{{"id": "kr://acme/catalog"}}})
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRetiredCatalogUseDoesNotPersistClientContext(t *testing.T) {
	isolateLoginConfig(t)
	server := loginTestServer(t, "alice", nil)
	result := Run([]string{"login", "--mode", "token", "--server", server.URL, "--token", "secret"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	use := Run([]string{"catalog", "use", "old", "--server", server.URL})
	if use.Status == 0 || !strings.Contains(use.Stdout, "USAGE_INVALID") {
		t.Fatal(use.Stdout)
	}
	if _, err := os.Stat(clientconfig.SessionPath(server.URL, "catalog.json")); !os.IsNotExist(err) {
		t.Fatalf("retired selection created context: %v", err)
	}
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
	if got := inv.RemoteServerURL(nil); got != serverB.URL {
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
	seedLoginState(t, server.URL, "session-taihu.json", map[string]any{"server": server.URL, "principal": "alice", "access_token": "old", "refresh_token": "refresh-old", "expires_at": time.Now().Add(-time.Minute)})
	if result := Run([]string{"whoami"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if result := Run([]string{"whoami"}); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if refreshCalls != 1 || whoamiCalls != 3 {
		t.Fatalf("refresh calls=%d whoami=%d", refreshCalls, whoamiCalls)
	}
	raw, err := os.ReadFile(clientconfig.SessionPath(server.URL, "session-taihu.json"))
	var saved map[string]any
	if err == nil {
		err = json.Unmarshal(raw, &saved)
	}
	if err != nil || saved["access_token"] != "fresh" || saved["refresh_token"] != "refresh-new" {
		t.Fatalf("refreshed login was not saved: error=%v", err)
	}
}

func TestExpiredSavedLoginNeverFallsBackToLocal(t *testing.T) {
	isolateLoginConfig(t)
	server := loginTestServer(t, "unexpected", func(*http.Request) { t.Error("expired token made a request") })
	seedLoginState(t, server.URL, "session-taihu.json", map[string]any{"server": server.URL, "principal": "alice", "access_token": "expired", "expires_at": time.Now().Add(-time.Minute)})
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
		if r.URL.Path == "/oauth2/par/poll" {
			_, _ = io.WriteString(w, `{"status":"completed","code":"one-time-code","redirect_uri":"http://localhost/callback"}`)
			return
		}
		_, secret, _ := r.BasicAuth()
		if secret != "server-secret" {
			t.Error("server did not provide its application secret")
		}
		_, _ = io.WriteString(w, `{"access_token":"verified-user-token","refresh_token":"refresh","expires_in":3600}`)
	}))
	t.Cleanup(upstream.Close)
	server := httptest.NewServer(serv.HTTPHandlerWithOptions(t.TempDir(), serv.HTTPServerOptions{Authenticator: testTaihuAuthenticator("kaiqidong"), LoginBroker: taihu.NewBroker(taihu.Application{ClientID: "kc-app", ClientSecret: "server-secret", OAuth2Base: upstream.URL}, nil)}))
	t.Cleanup(server.Close)
	seedLoginState(t, server.URL, "pending-taihu-auth.json", map[string]any{"request_uri": "urn:test:pending", "code_verifier": strings.Repeat("a", 43), "client_id": "kc-app", "resource": "kc-app", "oauth2_base": upstream.URL, "server": server.URL})
	result := Run([]string{"login", "--server", server.URL, "--wait"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if strings.Contains(result.Stdout, "verified-user-token") || strings.Contains(result.Stdout, "server-secret") {
		t.Fatal("login output disclosed a secret")
	}

	if result := Run([]string{"whoami"}); result.Status != 0 || !strings.Contains(result.Stdout, `"kaiqidong"`) {
		t.Fatalf("verified browser login did not continue: %s", result.Stdout)
	}
}

func TestLegacyTaihuTokenExchangeRequiresClientSecretAndSurfacesError(t *testing.T) {
	pending := legacyPendingAuth{
		ClientID:     "knowledge-catalog",
		CodeVerifier: "verifier",
		OAuth2Base:   "http://example.invalid",
	}
	if _, err := legacyExchangeTaihuAccessToken(pending, "code", "http://127.0.0.1:7382", "", nil); err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
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

	if _, err := legacyExchangeTaihuAccessToken(pending, "code", "http://127.0.0.1:7382", "wrong", ts.Client()); err == nil || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("wrong secret must surface Taihu error, got %v", err)
	}
	got, err := legacyExchangeTaihuAccessToken(pending, "code", "http://127.0.0.1:7382", "app-secret", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !sawSecret || got.AccessToken != "live-token" {
		t.Fatalf("token %#v sawSecret=%v", got, sawSecret)
	}
}

func TestCurrentTokenBrokerUsesDeploymentSecretAndRejectsInvalidProof(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("unexpected token endpoint: %s", r.URL.Path)
		}
		user, secret, ok := r.BasicAuth()
		if !ok || user != "knowledge-catalog" || secret != "app-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"Taihu app authentication failed"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "live-token", "expires_in": 60})
	}))
	t.Cleanup(upstream.Close)
	for _, test := range []struct {
		name   string
		secret string
		code   kernel.ErrorCode
		calls  int
	}{
		{"missing", "", kernel.ErrTemporaryUnavailable, 0},
		{"wrong", "wrong", kernel.ErrUnauthenticated, 1},
		{"accepted", "app-secret", "", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := upstreamCalls
			handler := serv.HTTPHandlerWithOptions(t.TempDir(), serv.HTTPServerOptions{Authenticator: testTaihuAuthenticator("kaiqidong"), LoginBroker: taihu.NewBroker(taihu.Application{ClientID: "knowledge-catalog", ClientSecret: test.secret, OAuth2Base: upstream.URL}, nil)})
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			remote, err := kcclient.New(kcclient.Config{BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			result, err := remote.IdentityService().ExchangeToken(context.Background(), kcclient.TokenRequest{GrantType: "authorization_code", Code: "code", CodeVerifier: strings.Repeat("a", 43), RedirectURI: "http://127.0.0.1:7382"})
			if test.code == "" {
				if err != nil || result.AccessToken != "live-token" {
					t.Fatalf("accepted deployment secret did not return token: %v", err)
				}
			} else if kernel.CodeOf(err) != test.code {
				t.Fatalf("broker did not reject %s deployment secret: %v", test.name, err)
			}
			if upstreamCalls-before != test.calls {
				t.Fatalf("broker contacted upstream %d times, want %d", upstreamCalls-before, test.calls)
			}
		})
	}
}

func TestEmbeddedExplicitHomeIgnoresClientDefaultsWithoutChangingProductBoundary(t *testing.T) {
	isolateLoginConfig(t)
	home := t.TempDir()
	if result := RunEmbeddedForTest([]string{"--home", home, "local", "init"}, nil); result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		t.Error("explicit embedded Home unexpectedly contacted a default Server")
		w.WriteHeader(500)
	}))
	defer server.Close()
	seedLoginState(t, server.URL, "session-local.json", map[string]any{"server": server.URL, "principal": "agent:remote"})
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

// Fixtures use the persisted wire contract without exposing session internals.
func seedLoginState(t *testing.T, server, name string, state any) {
	t.Helper()
	if err := jsonfile.WritePrivate(clientconfig.SessionPath(server, name), state); err != nil {
		t.Fatal(err)
	}
	if err := jsonfile.WritePrivate(filepath.Join(clientconfig.ConfigDir(), "client.json"), map[string]string{"server": strings.TrimRight(server, "/")}); err != nil {
		t.Fatal(err)
	}
}

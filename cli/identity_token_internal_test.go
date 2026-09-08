package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kcclient "kc/client"
)

func TestIdentityTokenBrokerKeepsApplicationSecretOnServer(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("wrong endpoint %s", r.URL.Path)
		}
		_ = r.ParseForm()
		id, secret, ok := r.BasicAuth()
		if !ok || id != "kc-app" || secret != "server-secret" || r.Form.Get("client_id") != "kc-app" || r.Form.Get("client_secret") != "server-secret" {
			t.Error("missing fixed application identity")
		}
		if r.Form.Get("grant_type") == "authorization_code" && (r.Form.Get("code") != "one-time-code" || r.Form.Get("code_verifier") != strings.Repeat("a", 43)) {
			t.Error("PKCE proof lost")
		}
		if r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") != "refresh-credential" {
			t.Error("refresh proof lost")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "user-credential", "refresh_token": "rotated-refresh", "expires_in": 3600})
	}))
	defer upstream.Close()
	srv := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{Authenticator: pairingAuthenticator{principal: "kaiqidong"}, ServiceIdentity: &ServiceIdentity{ClientID: "kc-app", ClientSecret: "server-secret", OAuth2Base: upstream.URL}}))
	defer srv.Close()
	client, err := kcclient.New(kcclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := client.IdentityService().Discover(context.Background())
	if err != nil || discovery.BrowserLogin == nil {
		t.Fatalf("broker not discoverable: %+v %v", discovery, err)
	}
	raw, _ := json.Marshal(discovery)
	if bytes.Contains(raw, []byte("server-secret")) {
		t.Fatal("secret published in discovery")
	}
	for _, input := range []kcclient.TokenRequest{
		{GrantType: "authorization_code", Code: "one-time-code", CodeVerifier: strings.Repeat("a", 43), RedirectURI: "http://127.0.0.1/callback"},
		{GrantType: "refresh_token", RefreshToken: "refresh-credential"},
	} {
		result, err := client.IdentityService().ExchangeToken(context.Background(), input)
		if err != nil || result.AccessToken != "user-credential" || result.ExpiresIn != 3600 {
			t.Fatalf("exchange failed: %v", err)
		}
	}
	if calls != 2 {
		t.Fatalf("upstream calls %d", calls)
	}
	resp, err := http.Post(srv.URL+"/identity/v1/token", "application/json", strings.NewReader(`{"grantType":"refresh_token","refreshToken":"x","oauth2Base":"http://attacker","clientSecret":"other"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || calls != 2 {
		t.Fatalf("caller provisioning fields accepted: status %d calls %d", resp.StatusCode, calls)
	}
}

func TestIdentityTokenBrokerRejectsUpstreamFailureAndRedirect(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1:1/leak")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"access_token":"should-never-be-returned","expires_in":3600,"error_description":"server-secret"}`))
			}))
			defer upstream.Close()
			srv := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{Authenticator: pairingAuthenticator{principal: "kaiqidong"}, ServiceIdentity: &ServiceIdentity{ClientID: "kc-app", ClientSecret: "server-secret", OAuth2Base: upstream.URL}}))
			defer srv.Close()
			client, _ := kcclient.New(kcclient.Config{BaseURL: srv.URL})
			result, err := client.IdentityService().ExchangeToken(context.Background(), kcclient.TokenRequest{GrantType: "refresh_token", RefreshToken: "refresh"})
			if err == nil || result.AccessToken != "" || strings.Contains(err.Error(), "server-secret") {
				t.Fatalf("failure leaked or accepted: %v", err)
			}
		})
	}
}

func TestBrowserAuthorizationUsesFixedDeploymentUpstream(t *testing.T) {
	seen := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "kc-app" || r.Header.Get("Authorization") != "" {
			t.Error("browser authorization must use fixed public application identity")
		}
		switch r.URL.Path {
		case "/oauth2/par":
			if r.Form.Get("code_challenge") == "" {
				t.Error("PKCE challenge missing")
			}
			_, _ = w.Write([]byte(`{"request_uri":"urn:authorize:one","expires_in":300}`))
		case "/oauth2/par/poll":
			if r.Form.Get("request_uri") != "urn:authorize:one" {
				t.Error("request URI missing")
			}
			_, _ = w.Write([]byte(`{"status":"completed","code":"one-time-code","redirect_uri":"http://127.0.0.1/callback"}`))
		default:
			t.Error("unexpected upstream path")
		}
	}))
	defer upstream.Close()
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{Authenticator: pairingAuthenticator{principal: "kaiqidong"}, ServiceIdentity: &ServiceIdentity{ClientID: "kc-app", ClientSecret: "server-secret", OAuth2Base: upstream.URL}})
	defer handler.(interface{ Close() error }).Close()
	for _, step := range []struct{ path, body string }{
		{"/identity/v1/authorize", `{"codeChallenge":"` + strings.Repeat("a", 43) + `","state":"` + strings.Repeat("b", 43) + `"}`},
		{"/identity/v1/authorize:poll", `{"requestURI":"urn:authorize:one"}`},
	} {
		r := httptest.NewRequest(http.MethodPost, step.path, strings.NewReader(step.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "server-secret") {
			t.Fatalf("browser flow unavailable: %d %s", w.Code, w.Body.String())
		}
	}
	if len(seen) != 2 {
		t.Fatalf("steps not relayed: %v", seen)
	}
}

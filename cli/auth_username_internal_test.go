package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"

	"kc/identity"
)

func TestGiteaUserPrincipalIsVerifiedUsername(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-credential" {
			t.Error("credential did not reach the trusted verifier")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "login": "kaiqidong", "active": true})
	}))
	defer provider.Close()
	authenticator, err := NewGiteaAuthenticator(provider.URL, provider.Client())
	if err != nil {
		t.Fatal(err)
	}
	id, err := authenticator.Authenticate(context.Background(), http.Header{"Authorization": []string{"Bearer user-credential"}})
	if err != nil {
		t.Fatal(err)
	}
	if id.Principal != "kaiqidong" || id.Login != "kaiqidong" || id.Subject != "42" {
		t.Fatalf("verified user must have a username principal and a separate stable subject: %#v", id)
	}
}

func TestTaihuUnsignedGatewayCannotBindAUsername(t *testing.T) {
	authenticator, err := NewTaihuAuthenticator("6b632d746573742d686d6163", "", "kc", "", nil)
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
}

func TestTaihuGatewayRequiresConfiguredTrustedIssuer(t *testing.T) {
	secret := []byte("kc-test-hmac")
	a, err := NewTaihuAuthenticator("6b632d746573742d686d6163", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set("X-Tai-Identity", signedTaihuIdentity(secret, `{"staff_id":"42","user_name":"kaiqidong"}`))
	if got, err := a.Authenticate(context.Background(), headers); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("signed identity without declared issuer occupied a username: %#v %v", got, err)
	}
}

func TestServerUsernameBindingSurvivesRestartAndRejectsRecycledAccount(t *testing.T) {
	home := t.TempDir()
	login, subject := "kaiqidong", int64(42)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": subject, "login": login, "active": true})
	}))
	defer provider.Close()
	newServer := func(origin string) *httptest.Server {
		t.Helper()
		auth, err := NewGiteaAuthenticator(origin, provider.Client())
		if err != nil {
			t.Fatal(err)
		}
		handler := HTTPHandlerWithOptions(home, HTTPServerOptions{Authenticator: auth})
		t.Cleanup(func() { _ = handler.(interface{ Close() error }).Close() })
		return httptest.NewServer(handler)
	}
	headers := http.Header{"Authorization": []string{"Bearer user-credential"}}
	server := newServer(provider.URL)
	status, body := pairingGET(t, server, "/identity/v1/whoami", headers)
	if status != http.StatusOK || body["principal"] != "kaiqidong" || body["login"] != "kaiqidong" {
		t.Fatalf("first verified username: %d %#v", status, body)
	}
	if _, ok := body["subject"]; ok {
		t.Fatal("whoami exposed provider subject")
	}
	if _, ok := body["provider"]; ok {
		t.Fatal("whoami exposed provider identifier")
	}
	server.Close()
	server = newServer(provider.URL)
	defer server.Close()
	status, _ = pairingGET(t, server, "/identity/v1/whoami", headers)
	if status != http.StatusOK {
		t.Fatalf("restored account: %d", status)
	}
	subject = 43
	status, _ = pairingGET(t, server, "/identity/v1/whoami", headers)
	if status != http.StatusForbidden {
		t.Fatalf("recycled username took old identity: %d", status)
	}
	subject = 42
	login = "renamed"
	status, _ = pairingGET(t, server, "/identity/v1/whoami", headers)
	if status != http.StatusForbidden {
		t.Fatalf("rename changed authorization key: %d", status)
	}
	login = "kaiqidong"
	if err := os.Remove(filepath.Join(home, identity.Filename)); err != nil {
		t.Fatal(err)
	}
	status, _ = pairingGET(t, server, "/identity/v1/whoami", headers)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("missing identity state was silently rebuilt: %d", status)
	}
}

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/identity"
)

// TestLiveTaihuAuthentication talks to real Taihu introspection. It is
// authentication only: whoami and pairing headers, not allow.json.
//
//	KC_LIVE_TAIHU=1 KC_SERVICE_CLIENT_SECRET=… KC_AUTH_TOKEN=… go test -count=1 -run TestLiveTaihuAuthentication ./cli
func TestLiveTaihuAuthentication(t *testing.T) {
	if strings.TrimSpace(os.Getenv("KC_LIVE_TAIHU")) != "1" {
		t.Skip("set KC_LIVE_TAIHU=1, KC_SERVICE_CLIENT_SECRET, and KC_AUTH_TOKEN to verify real Taihu introspection")
	}
	secret := strings.TrimSpace(os.Getenv("KC_SERVICE_CLIENT_SECRET"))
	token := strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN"))
	if secret == "" || token == "" {
		t.Fatal("KC_LIVE_TAIHU=1 requires KC_SERVICE_CLIENT_SECRET and KC_AUTH_TOKEN")
	}
	authURL := strings.TrimSpace(os.Getenv("KC_TAIHU_AUTH_URL"))
	if authURL == "" {
		authURL = "http://iam.it.woa.com"
	}
	clientID := strings.TrimSpace(os.Getenv("KC_TAIHU_CLIENT_ID"))
	if clientID == "" {
		clientID = "knowledge-catalog"
	}

	authenticator, err := NewTaihuAuthenticator("", authURL, clientID, secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{
		AuthMode:      "taihu",
		Authenticator: authenticator,
	}))
	t.Cleanup(server.Close)

	status, body := pairingGET(t, server, "/identity/v1/auth", nil)
	if status != http.StatusOK || body["mode"] != "taihu" || body["localAssertion"] != false {
		t.Fatalf("live discovery: %d %#v", status, body)
	}

	status, _ = pairingGET(t, server, "/identity/v1/whoami", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("empty identity: %d", status)
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", http.Header{"X-Kc-As": []string{"agent:dsh"}})
	if status != http.StatusUnauthorized || !strings.Contains(fmtErr(body), "pairing mismatch") {
		t.Fatalf("X-Kc-As against Taihu: %d %#v", status, body)
	}

	authorization := token
	if !strings.Contains(authorization, " ") {
		authorization = "Bearer " + authorization
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", http.Header{"Authorization": []string{authorization}})
	if status != http.StatusOK {
		t.Fatalf("Bearer whoami: %d %#v", status, body)
	}
	principal, _ := body["principal"].(string)
	onBehalf, _ := body["onBehalfOf"].(string)
	if onBehalf == "" {
		if !strings.HasPrefix(principal, "service:") {
			if _, err := identity.CanonicalUsername(principal); err != nil {
				t.Fatalf("user principal must be a canonical username: %q %v", principal, err)
			}
			login, _ := body["login"].(string)
			if login == "" || principal != login {
				t.Fatalf("whoami login must match username principal: %#v", body)
			}
		}
	} else {
		if !strings.HasPrefix(principal, "agent:") {
			t.Fatalf("delegated caller is not an agent: %q", principal)
		}
		if _, err := identity.CanonicalUsername(onBehalf); err != nil {
			t.Fatalf("delegated user must be a canonical username: %q %v", onBehalf, err)
		}
	}
	if _, ok := body["subject"]; ok {
		t.Fatal("whoami must not expose the provider subject")
	}
	if _, ok := body["provider"]; ok {
		t.Fatal("whoami must not expose the provider identifier")
	}

	status, _ = pairingGET(t, server, "/identity/v1/whoami", http.Header{
		"Authorization": []string{authorization},
		"X-Kc-As":       []string{"agent:forged"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("mixed headers: %d", status)
	}
}

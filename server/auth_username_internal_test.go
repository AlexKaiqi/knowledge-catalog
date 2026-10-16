package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/identity"
	"kc/identity/taihu"
)

func TestServerUsernameBindingSurvivesRestartAndRejectsRecycledAccount(t *testing.T) {
	home := t.TempDir()
	login, subject := "kaiqidong", "42"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "subject": subject, "username": login})
	}))
	defer provider.Close()
	newServer := func(origin string) *httptest.Server {
		t.Helper()
		auth, err := taihu.New("", origin, "", "", provider.Client())
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
	subject = "43"
	status, _ = pairingGET(t, server, "/identity/v1/whoami", headers)
	if status != http.StatusForbidden {
		t.Fatalf("recycled username took old identity: %d", status)
	}
	subject = "42"
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

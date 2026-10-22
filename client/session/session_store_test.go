package session

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/internal/clientconfig"
	"kc/kernel"
)

func isolateLoginConfig(t *testing.T) {
	t.Helper()
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_SERVER_URL", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_AUTH_TOKEN", "")
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
			result, err := exchangeTaihuCode(context.Background(), taihuPendingAuth{OAuth2Base: server.URL, Server: server.URL, ClientID: "fixture", CodeVerifier: "verifier"}, "code", "http://localhost/callback")
			if err == nil {
				t.Fatalf("LOGIN-VERIFIED-DURABLE: %s failure returned success: %#v", failure, result)
			}
		})
	}
}
func TestTaihuSessionPersistRoundTrip(t *testing.T) {
	isolateLoginConfig(t)
	const server = "http://localhost:7380"
	path := clientconfig.SessionPath(server, "session-taihu.json")

	// Not present -> not ok.
	if _, ok, err := currentServerTokenSession(context.Background(), server); ok || err != nil {
		t.Fatalf("load of missing session must report ok=false without error: %v", err)
	}

	sess := taihuSession{
		Server: server, AccessToken: "test-access-token",
		RefreshToken: "test-refresh-token", ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := persistTokenLogin(sess); err != nil {
		t.Fatal(err)
	}
	got, ok, err := currentServerTokenSession(context.Background(), server)
	if !ok || err != nil {
		t.Fatalf("load of persisted session must be ok: %v", err)
	}
	if got.Server != sess.Server || got.AccessToken != sess.AccessToken || got.RefreshToken != sess.RefreshToken {
		t.Fatalf("round-trip mismatch: %#v", got)
	}

	// An expired credential without a refresh token is not usable.
	expired := taihuSession{Server: sess.Server, AccessToken: "x", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := persistTokenLogin(expired); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := currentServerTokenSession(context.Background(), server); ok || kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("expired session must reject authentication: ok=%v err=%v", ok, err)
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
	info, err := os.Stat(clientconfig.SessionPath("http://kc-server:7380", "session-local.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("session file must be 0600, got %o", perm)
	}
}

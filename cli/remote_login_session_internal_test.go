package cli

import (
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

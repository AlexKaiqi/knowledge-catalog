package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
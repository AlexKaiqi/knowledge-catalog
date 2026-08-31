package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTaihuIntrospectionURLIsSingleEndpoint guards against the double-append
// bug where the introspection URL got "/oauth2/introspect" concatenated twice
// (base construction + per-request), producing a 404. The server must receive
// exactly one "/oauth2/introspect" request path.
func TestTaihuIntrospectionURLIsSingleEndpoint(t *testing.T) {
	var receivedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active":   true,
			"sub":      "12345",
			"subject":  "12345",
			"username": "alice",
		})
	}))
	defer ts.Close()

	a, err := NewTaihuAuthenticator("", ts.URL, "knowledge-catalog", "secret", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if a.introspectionURL != ts.URL+"/oauth2/introspect" {
		t.Fatalf("introspectionURL = %q, want %q", a.introspectionURL, ts.URL+"/oauth2/introspect")
	}

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer some-token")
	id, err := a.Authenticate(context.Background(), hdr)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if receivedPath != "/oauth2/introspect" {
		t.Fatalf("introspection request path = %q, want exactly /oauth2/introspect (no double append)", receivedPath)
	}
	if id.Principal != "taihu:12345" {
		t.Fatalf("principal = %q, want taihu:12345", id.Principal)
	}
	if id.Subject != "12345" {
		t.Fatalf("subject = %q, want 12345", id.Subject)
	}
}

// TestTaihuIntrospectionBaseHandlesOAuth2Suffix verifies that passing a base
// URL that already ends with /oauth2 does not produce a doubled path.
func TestTaihuIntrospectionBaseHandlesOAuth2Suffix(t *testing.T) {
	a, err := NewTaihuAuthenticator("", "http://iam.example/oauth2", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.introspectionURL != "http://iam.example/oauth2/introspect" {
		t.Fatalf("introspectionURL = %q, want http://iam.example/oauth2/introspect", a.introspectionURL)
	}
}

// TestAuthHmacSecretFlagReadsDedicatedFlag verifies the HMAC secret now comes
// from --auth-hmac-secret, not the overloaded --auth-subject. A valid hex
// secret in --auth-hmac-secret must be accepted; an invalid hex value must be
// rejected with a clear error.
func TestAuthHmacSecretFlagReadsDedicatedFlag(t *testing.T) {
	// Valid hex secret via the dedicated flag is accepted.
	auth, err := resolveAuthenticator("taihu", map[string]FlagValue{
		"auth-hmac-secret": "6a8c6bf5a1e2",
	})
	if err != nil {
		t.Fatalf("valid --auth-hmac-secret must be accepted: %v", err)
	}
	ta, ok := auth.(*TaihuAuthenticator)
	if !ok {
		t.Fatalf("expected *TaihuAuthenticator, got %T", auth)
	}
	if len(ta.hmacSecret) == 0 {
		t.Fatal("hmac secret must be decoded")
	}

	// Invalid hex via the dedicated flag is rejected.
	if _, err := resolveAuthenticator("taihu", map[string]FlagValue{
		"auth-hmac-secret": "not-hex!!",
	}); err == nil {
		t.Fatal("invalid hex --auth-hmac-secret must fail")
	} else if !strings.Contains(err.Error(), "hex-encoded") {
		t.Fatalf("expected hex decode error, got: %v", err)
	}
}

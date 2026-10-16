package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/identity"
	"kc/identity/identitytest"
	"kc/kernel"
	"kc/snapshot/access"
)

var nativeExternalTestIdentity = identitytest.Bearer("taihu", map[string]identity.HTTPIdentity{
	"Bearer alice-user-token": {Principal: "alice", Admin: true, User: &identity.VerifiedUser{Username: "alice", Provider: "taihu", Issuer: "https://identity.test", Subject: "123"}},
})

func externalNativeFixture(t *testing.T, actions ...string) (Handler, *nativeTestUpstream) {
	t.Helper()
	upstream := &nativeTestUpstream{}
	backend := httptest.NewServer(upstream)
	t.Cleanup(backend.Close)
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: actions}}}
	h := NewHandler(Config{NativeOrigin: backend.URL, NativeCredential: func(context.Context) (string, error) { return "storage-key:private-storage-secret", nil },
		Identity: IdentityOptions{Authenticator: nativeExternalTestIdentity, ProviderURL: "https://identity.test"},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
			return Resources{NativeRepositories: map[string]kernel.RepositoryID{"alpha": "kr://test/a"}, Access: access.NewService(rules)}, func() {}, nil
		}})
	return h, upstream
}

func externalNativeCall(t *testing.T, h http.Handler, method, path string, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer alice-user-token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return w
}

func TestNativeServiceProofPreservesExternalCallerAndPrivateStorageCredential(t *testing.T) {
	h, backend := externalNativeFixture(t, access.RepositoryFileRead)
	w := externalNativeCall(t, h, "GET", "/api/v1/repositories/alpha/refs/main/objects?path=a", 200)
	if len(backend.proof) != 1 || strings.Contains(backend.proof[0], "alice") || !strings.HasPrefix(backend.proof[0], "Basic ") {
		t.Fatalf("caller proof reached storage: %#v", backend.proof)
	}
	if strings.Contains(w.Body.String(), "private-storage-secret") {
		t.Fatal("storage credential reached caller")
	}
	externalNativeCall(t, h, "POST", "/api/v1/repositories/alpha/branches/main/commits", 403)
	if len(backend.calls) != 1 {
		t.Fatal("denied mutation reached storage")
	}
}

func TestNativeServiceAccountsCannotExposeStorageAdministrator(t *testing.T) {
	h, backend := externalNativeFixture(t, "*")
	for _, path := range []string{"/api/v1/auth/users", "/api/v1/auth/users/storage-key/credentials", "/api/v1/auth/groups", "/api/v1/auth/policies", "/api/v1/statistics"} {
		externalNativeCall(t, h, "GET", path, 403)
	}
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/setup_lakefs", "/api/v1/setup_comm_prefs"} {
		externalNativeCall(t, h, "POST", path, 403)
	}
	w := externalNativeCall(t, h, "GET", "/api/v1/user", 200)
	var profile struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if json.Unmarshal(w.Body.Bytes(), &profile) != nil || profile.User.ID != "alice" || len(backend.calls) != 0 {
		t.Fatalf("native menu used storage account: %s %#v", w.Body.String(), backend.calls)
	}
}

func TestNativeExternalStartupAndMissingStorageProof(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("startup leaked proof")
		}
		writeJSON(w, 200, map[string]any{"state": "initialized"})
	}))
	defer backend.Close()
	h := NewHandler(Config{NativeOrigin: backend.URL, Identity: IdentityOptions{Authenticator: nativeExternalTestIdentity, LogoutURL: "https://identity.test/end-session"}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/setup_lakefs", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://identity.test/end-session") {
		t.Fatalf("public startup failed: %d %s", w.Code, w.Body.String())
	}
	externalNativeCall(t, h, "GET", "/api/v1/user", 200)
	externalNativeCall(t, h, "GET", "/api/v1/repositories/alpha", 503)
	externalNativeCall(t, h, "GET", "/api/v1/config", 503)
	if calls != 1 {
		t.Fatalf("missing credential dispatched a private request: %d", calls)
	}
}

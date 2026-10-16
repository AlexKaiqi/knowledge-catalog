package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	apphome "kc/home"
	"kc/identity"
	"kc/identity/identitytest"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/snapshot"
	"kc/snapshot/access"
)

// verifiedCallerTestIdentity stands in for an external identity provider that
// accepts the caller's own credential.
var verifiedCallerTestIdentity = identitytest.Bearer("verified", map[string]identity.HTTPIdentity{"Bearer caller-proof": {Principal: "alice"}})

func localNativeFacadeForTest(t *testing.T, mode, credential string, granted bool, upstream http.Handler) (*httpFacade, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); upstream.ServeHTTP(w, r) }))
	t.Cleanup(backend.Close)
	t.Setenv(snapshot.LakeFSCredentialEnv, credential)
	dir := t.TempDir()
	source := testkit.MakeTreeStore(t, "kr://fixture/native")
	stores := snapshot.NewRegistry()
	if err := stores.Add(source); err != nil {
		t.Fatal(err)
	}
	policy := inv.AllowFile{}
	if granted {
		policy.Rules = []access.Rule{{ID: "native-writer", Principal: "alice", Repo: string(source.ID()), Actions: []string{access.RepositoryFilesWrite, access.RepositoryCommit}}}
	}
	if err := inv.WriteAllow(dir, policy); err != nil {
		t.Fatal(err)
	}
	opened := &Home{Dir: dir, Store: stores, File: apphome.HomeFile{Repos: []apphome.HomeRepo{{ID: string(source.ID()), Driver: "lakefs", DSN: backend.URL + "/alpha"}}}}
	return &httpFacade{home: dir, readHome: opened, options: HTTPServerOptions{AuthMode: mode, NativeAPIOrigin: backend.URL}}, &calls
}

func TestExplicitLocalNativeFacadeSeparatesCallerFromPrivateStorageProof(t *testing.T) {
	facade, calls := localNativeFacadeForTest(t, "local", "storage-key:private-test-secret", true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, secret, ok := r.BasicAuth()
		if !ok || key != "storage-key" || secret != "private-test-secret" || r.Header.Get("X-Kc-As") != "" || r.Header.Get("Cookie") != "" {
			t.Error("local caller identity replaced or leaked into storage proof")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"published"}`))
	}))
	handler := facade.snapshotStoreService()
	for _, target := range []string{"/api/v1/repositories/alpha/branches/main/objects?path=plain.txt", "/api/v1/repositories/alpha/branches/main/commits"} {
		grantRequestForTest(t, handler, "alice", http.MethodPost, target, `{}`, http.StatusOK)
	}
	if calls.Load() != 2 {
		t.Fatalf("granted local writes did not reach storage: %d", calls.Load())
	}
	profile := grantRequestForTest(t, handler, "alice", http.MethodGet, "/api/v1/user", "", http.StatusOK)
	if profile["user"].(map[string]any)["id"] != "alice" || calls.Load() != 2 {
		t.Fatalf("native profile used the storage account: %+v", profile)
	}
}

func TestLocalNativeFacadeRejectsMissingIdentityCredentialAndRepositoryGrant(t *testing.T) {
	for _, tc := range []struct {
		name, mode, credential, caller string
		grant                          bool
		status                         int
	}{
		{"anonymous", "local", "storage-key:private-test-secret", "", true, http.StatusUnauthorized},
		{"missing-storage-proof", "local", "", "alice", true, http.StatusServiceUnavailable},
		{"repository-denied", "local", "storage-key:private-test-secret", "alice", false, http.StatusForbidden},
		{"unconfigured", "unconfigured", "storage-key:private-test-secret", "alice", true, http.StatusUnauthorized},
		// The implicit seam has no private storage credential and never
		// falls back to caller proof.
		{"implicit-test-seam", "", "storage-key:private-test-secret", "alice", true, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facade, calls := localNativeFacadeForTest(t, tc.mode, tc.credential, tc.grant, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("rejected native request reached private storage") }))
			request := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/alpha/branches/main/commits", strings.NewReader(`{}`))
			if tc.caller != "" {
				request.Header.Set("X-Kc-As", tc.caller)
			}
			response := httptest.NewRecorder()
			facade.snapshotStoreService().ServeHTTP(response, request)
			if response.Code != tc.status || calls.Load() != 0 || strings.Contains(response.Body.String(), "private-test-secret") {
				t.Fatalf("native refusal changed: HTTP %d calls=%d", response.Code, calls.Load())
			}
		})
	}
}

func TestExplicitLocalNativeStartupUsesServiceIdentityWithoutVendorSetup(t *testing.T) {
	facade, calls := localNativeFacadeForTest(t, "local", "storage-key:private-test-secret", true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/setup_lakefs" {
			if r.Header.Get("Authorization") != "" {
				t.Error("public setup sent private storage proof")
			}
			_, _ = w.Write([]byte(`{"state":"initialized","comm_prefs_missing":true}`))
			return
		}
		key, secret, ok := r.BasicAuth()
		if !ok || key != "storage-key" || secret != "private-test-secret" {
			t.Error("verified local runtime config lost storage proof")
		}
		_, _ = w.Write([]byte(`{"version":"fixture","ui":{"enabled":true}}`))
	}))
	handler := facade.snapshotStoreService()
	setup := grantRequestForTest(t, handler, "", http.MethodGet, "/api/v1/setup_lakefs", "", http.StatusOK)
	if setup["state"] != "initialized" || setup["comm_prefs_missing"] != false {
		t.Errorf("local caller was sent to vendor Email User Configuration: %+v", setup)
	}
	login, _ := setup["login_config"].(map[string]any)
	if login["snapshot_identity"] != true || login["RBAC"] != "external" {
		t.Errorf("startup did not use configured local caller identity: %+v", login)
	}
	grantRequestForTest(t, handler, "", http.MethodGet, "/api/v1/config", "", http.StatusUnauthorized)
	grantRequestForTest(t, handler, "alice", http.MethodGet, "/api/v1/config", "", http.StatusOK)
	if calls.Load() != 2 {
		t.Fatalf("anonymous runtime config reached private storage: %d calls", calls.Load())
	}
}

// lakeFS is storage, not an identity provider: a verified caller's own
// credential (or a lakeFS session cookie) is never forwarded to it.
func TestVerifiedCallerProofIsNeverForwardedToStorage(t *testing.T) {
	facade, calls := localNativeFacadeForTest(t, "verified", "storage-key:private-test-secret", true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, secret, ok := r.BasicAuth()
		if !ok || key != "storage-key" || secret != "private-test-secret" || r.Header.Get("Cookie") != "" {
			t.Errorf("storage received caller proof: Authorization=%q Cookie=%q", r.Header.Get("Authorization"), r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "published"})
	}))
	facade.options.Authenticator = verifiedCallerTestIdentity
	request := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/alpha/branches/main/commits", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer caller-proof")
	request.Header.Set("Cookie", "internal_auth_session=caller-lakefs-session")
	response := httptest.NewRecorder()
	facade.snapshotStoreService().ServeHTTP(response, request)
	if response.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("verified caller write: HTTP %d calls=%d %s", response.Code, calls.Load(), response.Body.String())
	}
	facade.options.AuthMode = ""
	t.Setenv(snapshot.LakeFSCredentialEnv, "")
	response = httptest.NewRecorder()
	facade.snapshotStoreService().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatalf("missing service credential fell back to caller proof: HTTP %d calls=%d", response.Code, calls.Load())
	}
}

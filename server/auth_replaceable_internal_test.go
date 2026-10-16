package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	kcidentity "kc/identity"
	"kc/identity/identitytest"
)

const replaceableAuthName = "replaceable-test"

// Authentication is replaceable: an authenticator registered the way a new
// identity/<name> subpackage would register drives authentication,
// authorization and the console native proxy through unchanged Server code.
func TestRegisteredAuthenticatorReplacesProviderWithoutServerChange(t *testing.T) {
	identitytest.Register(replaceableAuthName, identitytest.Bearer(replaceableAuthName, map[string]kcidentity.HTTPIdentity{
		"Bearer alice-proof": {Principal: "alice"},
		"Bearer bob-proof":   {Principal: "bob"},
	}))
	options, err := HTTPServerOptionsFromFlags(map[string]FlagValue{"auth": replaceableAuthName})
	if err != nil || options.AuthMode != replaceableAuthName || options.Authenticator == nil || options.LoginBroker != nil {
		t.Fatalf("registered authenticator not selectable by --auth: %#v %v", options, err)
	}

	srv := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), options))
	t.Cleanup(srv.Close)
	status, body := pairingGET(t, srv, "/identity/v1/auth", nil)
	if status != http.StatusOK || body["mode"] != replaceableAuthName || body["localAssertion"] != false || !reflect.DeepEqual(body["accepts"], []any{"Authorization"}) {
		t.Fatalf("pairing discovery for replacement: %d %#v", status, body)
	}
	status, body = pairingGET(t, srv, "/identity/v1/whoami", http.Header{"Authorization": {"Bearer alice-proof"}})
	if status != http.StatusOK || body["principal"] != "alice" {
		t.Fatalf("replacement did not authenticate its proof: %d %s", status, fmtErr(body))
	}
	for _, headers := range []http.Header{nil, {"Authorization": {"Bearer forged"}}, {"X-Kc-As": {"alice"}}} {
		if status, body = pairingGET(t, srv, "/identity/v1/whoami", headers); status != http.StatusUnauthorized {
			t.Fatalf("replacement accepted %v: %d %s", headers, status, fmtErr(body))
		}
	}

	facade, calls := localNativeFacadeForTest(t, replaceableAuthName, "storage-key:private-test-secret", true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key, secret, ok := r.BasicAuth(); !ok || key != "storage-key" || secret != "private-test-secret" {
			t.Errorf("storage received caller proof instead of the service credential: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"published"}`))
	}))
	facade.options.Authenticator = options.Authenticator
	for _, tc := range []struct {
		proof  string
		status int
		calls  int32
	}{
		{"Bearer alice-proof", http.StatusOK, 1},
		{"Bearer bob-proof", http.StatusForbidden, 1},
		{"", http.StatusUnauthorized, 1},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/alpha/branches/main/commits", strings.NewReader(`{}`))
		if tc.proof != "" {
			request.Header.Set("Authorization", tc.proof)
		}
		response := httptest.NewRecorder()
		facade.snapshotStoreService().ServeHTTP(response, request)
		if response.Code != tc.status || calls.Load() != tc.calls {
			t.Fatalf("native proxy with %q: HTTP %d calls=%d %s", tc.proof, response.Code, calls.Load(), response.Body.String())
		}
	}
}

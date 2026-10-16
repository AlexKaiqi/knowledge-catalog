package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

func TestNativePhysicalSourcesRequireEverySourceOwner(t *testing.T) {
	dispatched := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/repositories" {
			writeJSON(w, 200, map[string]any{"results": []map[string]string{{"id": "alpha", "storage_namespace": "s3://bucket/a"}, {"id": "beta", "storage_namespace": "s3://bucket/b"}, {"id": "registry", "storage_namespace": "s3://bucket/private"}}, "pagination": map[string]bool{"has_more": false}})
			return
		}
		dispatched++
		w.WriteHeader(202)
	}))
	defer backend.Close()
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryImport, access.RepositoryFileRead}}}}
	h := NewHandler(Config{NativeOrigin: backend.URL, NativeCredential: func(context.Context) (string, error) { return "storage:secret", nil }, Identity: IdentityOptions{Authenticator: nativeExternalTestIdentity}, Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
		return Resources{NativeRepositories: map[string]kernel.RepositoryID{"alpha": "kr://test/a", "beta": "kr://test/b"}, Access: access.NewService(rules)}, nil, nil
	}})
	for _, row := range []struct {
		path, body string
		want       int
	}{
		{"/import", `{"paths":[{"path":"s3://bucket/a/data","type":"object"}],"commit":{"message":"owned"}}`, 202},
		{"/import", `{"paths":[{"path":"s3://bucket/b/data","type":"object"}]}`, 403},
		{"/import", `{"paths":[{"path":"s3://bucket/","type":"common_prefix"}]}`, 403},
		{"/import", `{"paths":[{"path":"s3://bucket/private/ledger","type":"object"}]}`, 403},
		{"/import", `{"paths":[{"path":"s3://bucket/a-lookalike/data","type":"object"}]}`, 403},
		{"/import", `{"paths":[{"path":"s3://bucket/a/%2e%2e/b/data","type":"object"}]}`, 403},
		{"/import", `{"paths":[{"path":"http://other/private","type":"object"}]}`, 403},
		{"/staging/backing?path=leak", `{"staging":{"physical_address":"s3://bucket/b/data"},"checksum":"a","size_bytes":1}`, 403},
	} {
		method := "POST"
		if strings.HasPrefix(row.path, "/staging") {
			method = "PUT"
		}
		r := httptest.NewRequest(method, "/api/v1/repositories/alpha/branches/main"+row.path, strings.NewReader(row.body))
		r.Header.Set("Authorization", "Bearer alice-user-token")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != row.want {
			t.Fatalf("%s => %d: %s", row.body, w.Code, w.Body.String())
		}
	}
	if dispatched != 1 {
		t.Fatalf("unauthorized physical requests dispatched: %d", dispatched)
	}
	rules.rules = append(rules.rules, access.Rule{Principal: "alice", Repo: "kr://test/b", Actions: []string{access.RepositoryFileRead}})
	r := httptest.NewRequest("POST", "/api/v1/repositories/alpha/branches/main/import", strings.NewReader(`{"paths":[{"path":"s3://bucket/b/data","type":"object"}]}`))
	r.Header.Set("Authorization", "Bearer alice-user-token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 202 || dispatched != 2 {
		t.Fatalf("authorized source denied: %d", w.Code)
	}
}

func TestNativeServiceBackendFailureIsObservedWithoutReplacingCaller(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", "internal_auth_session=private; Path=/")
				w.WriteHeader(status)
			}))
			defer backend.Close()
			var observed error
			var principal string
			rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryMetadataRead}}}}
			h := NewHandler(Config{NativeOrigin: backend.URL, NativeCredential: func(context.Context) (string, error) { return "storage:secret", nil }, Identity: IdentityOptions{Authenticator: nativeExternalTestIdentity}, Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
				return Resources{NativeRepositories: map[string]kernel.RepositoryID{"alpha": "kr://test/a"}, Access: access.NewService(rules)}, nil, nil
			}, Observe: func(ctx context.Context, caller identity.HTTPIdentity, _ string) (context.Context, func(any, error)) {
				principal = caller.Principal
				return ctx, func(_ any, e error) { observed = e }
			}})
			want := status
			if status == 401 || status == 403 {
				want = 503
			}
			w := externalNativeCall(t, h, "GET", "/api/v1/repositories/alpha", want)
			if observed == nil || principal != "alice" || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("invalid failure evidence: %v %s %v", observed, principal, w.Header())
			}
		})
	}
}

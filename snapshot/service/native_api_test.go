package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"kc/identity"
	"kc/identity/identitytest"
	"kc/kernel"
	"kc/snapshot/access"
)

type nativeTestRules struct {
	sync.Mutex
	rules []access.Rule
}

func (s *nativeTestRules) ReadRules(context.Context) ([]access.Rule, error) {
	s.Lock()
	defer s.Unlock()
	return append([]access.Rule(nil), s.rules...), nil
}
func (s *nativeTestRules) UpdateRules(_ context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	s.Lock()
	defer s.Unlock()
	rules, err := update(s.rules)
	if err == nil {
		s.rules = rules
	}
	return err
}

type nativeTestUpstream struct {
	sync.Mutex
	calls   []string
	bodies  []string
	proof   []string
	cookies []string
	query   []string
}

func (s *nativeTestUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/user" {
		name, _, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, map[string]any{"user": map[string]any{"id": name, "creation_date": 123}})
		return
	}
	raw, _ := io.ReadAll(r.Body)
	s.Lock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	s.bodies = append(s.bodies, string(raw))
	s.proof = append(s.proof, r.Header.Get("Authorization"))
	s.cookies = append(s.cookies, r.Header.Get("Cookie"))
	s.query = append(s.query, r.URL.RawQuery)
	s.Unlock()
	if r.URL.Path == "/api/v1/repositories" {
		results := []map[string]string{{"id": "hidden"}, {"id": "alpha"}, {"id": "beta"}}
		if after := r.URL.Query().Get("after"); after != "" {
			for i, row := range results {
				if row["id"] == after {
					results = results[i+1:]
					break
				}
			}
		}
		writeJSON(w, 200, map[string]any{"results": results, "pagination": map[string]any{"has_more": false, "next_offset": ""}})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		writeJSON(w, 501, map[string]string{"message": "native account capability unavailable"})
		return
	}
	if r.URL.Path == "/api/v1/repositories/alpha/pulls/one" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]string{"source_branch": "private", "destination_branch": "main"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	if r.Method == http.MethodPost {
		w.WriteHeader(201)
	}
	_, _ = w.Write([]byte{'b', 0, 'y', 't', 'e', 's'})
}

func nativeTestHandler(t *testing.T, rules *nativeTestRules) (Handler, *nativeTestUpstream) {
	t.Helper()
	upstream := &nativeTestUpstream{}
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	return NewHandler(Config{NativeOrigin: server.URL, NativeCredential: func(context.Context) (string, error) { return nativeServiceCredential, nil }, Identity: IdentityOptions{Authenticator: basicVerifier}, Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
		return Resources{NativeRepositories: map[string]kernel.RepositoryID{"alpha": "kr://test/a", "beta": "kr://test/b", "hidden": "kr://test/hidden"}, Access: access.NewService(rules)}, func() {}, nil
	}}), upstream
}

// nativeServiceCredential is the Server's private storage key; callers never
// see it and their own proof never reaches storage.
const nativeServiceCredential = "kc-service:storage-secret"

func nativeServiceProof() string {
	r := &http.Request{Header: http.Header{}}
	r.SetBasicAuth("kc-service", "storage-secret")
	return r.Header.Get("Authorization")
}

// basicVerifier stands in for a deployment identity provider: it verifies the
// caller's Basic credential and names the caller by its user.
var basicVerifier = identitytest.Authenticator{Mode: "verified", Verify: func(headers http.Header) (identity.HTTPIdentity, error) {
	user, _, ok := (&http.Request{Header: headers}).BasicAuth()
	if !ok || user == "" {
		return identity.HTTPIdentity{}, identitytest.Reject("missing caller credential")
	}
	return identity.HTTPIdentity{Principal: user}, nil
}}

func callNativeTest(t *testing.T, handler http.Handler, user, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.SetBasicAuth(user, "caller-secret")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s as %s: want %d, got %d %s", method, path, user, want, response.Code, response.Body.String())
	}
	return response
}

func TestNativeRepoCurrentProductGrantsAndCallerProof(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryMetadataRead, access.RepositoryFileRead, access.RepositoryHistoryRead}}, {Principal: "bob", Repo: "kr://test/b", Actions: []string{access.RepositoryMetadataRead, access.RepositoryFileRead}}}}
	handler, upstream := nativeTestHandler(t, rules)
	for _, row := range []struct {
		user, path string
		want       int
	}{{"alice", "alpha", 200}, {"alice", "beta", 403}, {"bob", "alpha", 403}, {"bob", "beta", 200}, {"alice", "unregistered", 403}} {
		response := callNativeTest(t, handler, row.user, "GET", "/api/v1/repositories/"+row.path+"/refs/main/objects?path=data&presign=true", "", row.want)
		if row.want == 200 && response.Body.String() != string([]byte{'b', 0, 'y', 't', 'e', 's'}) {
			t.Fatal("native bytes changed")
		}
	}
	if len(upstream.calls) != 2 || !strings.Contains(upstream.query[0], "presign=false") {
		t.Fatalf("denied calls reached native or presigned delivery escaped: %#v %#v", upstream.calls, upstream.query)
	}
	for _, proof := range upstream.proof {
		if proof != nativeServiceProof() {
			t.Fatalf("storage received caller proof instead of the service credential: %q", proof)
		}
	}
	callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/alpha/branches/main/commits", "{}", 403)
	callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories/alpha/unreviewed-read", "", 403)
	callNativeTest(t, handler, "alice", "GET", "/api/v1/auth/groups", "", 403)
	if len(upstream.calls) != 2 {
		t.Fatalf("storage account API was relayed for a product caller: %#v", upstream.calls)
	}
	if err := rules.UpdateRules(context.Background(), func(rows []access.Rule) ([]access.Rule, error) { return rows[1:], nil }); err != nil {
		t.Fatal(err)
	}
	callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories/alpha/refs/main/objects?path=data", "", 403)
}

func TestNativeInventoryFiltersBeforePaginationAndDoesNotExposeHiddenCursor(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryMetadataRead}}, {Principal: "alice", Repo: "kr://test/b", Actions: []string{access.RepositoryMetadataRead}}}}
	handler, _ := nativeTestHandler(t, rules)
	response := callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories?amount=1", "", 200)
	var first nativeInventoryPage
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Results) != 1 || !first.Pagination.HasMore || first.Pagination.NextOffset != "alpha" || strings.Contains(response.Body.String(), "hidden") {
		t.Fatalf("filtered pagination leaked: %s", response.Body.String())
	}
	response = callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories?amount=1&after=alpha", "", 200)
	var second nativeInventoryPage
	_ = json.Unmarshal(response.Body.Bytes(), &second)
	if len(second.Results) != 1 || second.Pagination.HasMore || !strings.Contains(string(second.Results[0]), "beta") {
		t.Fatalf("authorized second page missing: %s", response.Body.String())
	}
}

func TestNativeDatasetAndObjectQualifiedGrantsDoNotOpenRepo(t *testing.T) {
	for _, rule := range []access.Rule{{Dataset: "data"}, {Repo: "kr://test/a", Object: "*"}, {Repo: "kr://test/a", Aspect: "metadata"}, {Dataset: "other"}} {
		rule.Principal = "alice"
		rule.Actions = []string{access.RepositoryFileRead, access.RepositoryMetadataRead}
		handler, upstream := nativeTestHandler(t, &nativeTestRules{rules: []access.Rule{rule}})
		callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories/alpha/refs/main/objects?path=data", "", 403)
		if len(upstream.calls) != 0 {
			t.Fatal("qualified grant expanded to native Repo")
		}
	}
}

func TestNativeCopyChecksSourceBeforeForwarding(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryFilesWrite}}, {Principal: "alice", Repo: "kr://test/a", Ref: "approved", Actions: []string{access.RepositoryFileRead}}}}
	handler, upstream := nativeTestHandler(t, rules)
	callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/alpha/branches/main/objects/copy", `{"src_ref":"private","src_path":"file"}`, 403)
	callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/alpha/branches/main/objects/copy", `{"src_ref":"approved","src_path":"file"}`, 201)
	callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/alpha/branches/main/objects/copy", `{"src_ref":"approved","source_repository":"beta","src_path":"file"}`, 403)
	if len(upstream.calls) != 1 {
		t.Fatal("source permission bypassed")
	}
}

func TestNativeMutationMethodsUseSpecificProductActions(t *testing.T) {
	for _, row := range []struct{ method, path, action string }{{"PUT", "/api/v1/repositories/alpha/settings/branch_protection", access.RepositorySettingsManage}, {"DELETE", "/api/v1/repositories/alpha/tags/old", access.RepositoryRefsManage}, {"PATCH", "/api/v1/repositories/alpha/pulls/one", access.RepositoryPullsManage}} {
		rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryFileRead}}}}
		handler, upstream := nativeTestHandler(t, rules)
		callNativeTest(t, handler, "alice", row.method, row.path, "{}", 403)
		_ = rules.UpdateRules(context.Background(), func(rows []access.Rule) ([]access.Rule, error) {
			rows[0].Actions = []string{row.action}
			return rows, nil
		})
		callNativeTest(t, handler, "alice", row.method, row.path, "{}", 200)
		if len(upstream.calls) != 1 || upstream.bodies[0] != "{}" {
			t.Fatal("native mutation was replayed or denied call forwarded")
		}
	}
}

func TestNativeUploadCSRFIsNarrowAndJSONContractsStayStrict(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryFilesWrite}}}}
	gateway, upstream := nativeTestHandler(t, rules)
	h := gateway.(*handler)
	h.config.Identity.Authenticator = &identitytest.Recorder{Mode: "verified", Caller: identity.HTTPIdentity{Principal: "alice"}}
	for _, site := range []string{"same-origin", "cross-site"} {
		r := httptest.NewRequest("POST", "/api/v1/repositories/alpha/branches/main/objects?path=data", strings.NewReader("multipart-exact-body"))
		r.Header = http.Header{"Cookie": {"internal_auth_session=caller-session"}, "X-Kc-Console": {"1"}, "Sec-Fetch-Site": {site}, "Content-Type": {"multipart/form-data; boundary=owned"}}
		w := httptest.NewRecorder()
		gateway.ServeHTTP(w, r)
		want := 201
		if site == "cross-site" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("upload CSRF %s: %d %s", site, w.Code, w.Body.String())
		}
		if site == "same-origin" {
			if _, err := AuthenticateRequest(r, h.config.Identity); kernel.CodeOf(err) != kernel.ErrForbidden {
				t.Fatal("native upload exception widened typed JSON routes")
			}
		}
	}
	if len(upstream.calls) != 1 || upstream.bodies[0] != "multipart-exact-body" || upstream.proof[0] != nativeServiceProof() || upstream.cookies[0] != "" {
		t.Fatalf("upload changed body or forwarded caller session: proof=%q cookie=%q", upstream.proof, upstream.cookies)
	}
}

func TestNativeRefParametersDecodeExactlyOnce(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Ref: "feature/branch", Actions: []string{access.RepositoryFileRead}}}}
	handler, _ := nativeTestHandler(t, rules)
	callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories/alpha/refs/feature%2Fbranch/objects?path=data", "", 200)
	callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories/alpha/refs/feature%252Fbranch/objects?path=data", "", 403)
}

func TestNativePRMergeRequiresCurrentSourceRead(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryMerge, access.RepositoryPullsRead}}, {Principal: "alice", Repo: "kr://test/a", Ref: "approved", Actions: []string{access.RepositoryHistoryRead}}}}
	gateway, upstream := nativeTestHandler(t, rules)
	callNativeTest(t, gateway, "alice", "PUT", "/api/v1/repositories/alpha/pulls/one/merge", "", 403)
	for _, call := range upstream.calls {
		if strings.Contains(call, "/merge") {
			t.Fatal("PR merge bypassed source authorization")
		}
	}
}

func TestNativeEncodedTraversalCannotBecomeAnAccountBypass(t *testing.T) {
	gateway, upstream := nativeTestHandler(t, &nativeTestRules{})
	callNativeTest(t, gateway, "alice", "GET", "/api/v1/auth/%2e%2e/repositories/alpha", "", 403)
	if len(upstream.calls) != 0 {
		t.Fatal("account prefix forwarded an encoded traversal")
	}
}

func TestNativeDeleteCannotOrphanRegisteredAuthority(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryDelete}}}}
	handler, upstream := nativeTestHandler(t, rules)
	callNativeTest(t, handler, "alice", "DELETE", "/api/v1/repositories/alpha", "", 409)
	if len(upstream.calls) != 0 {
		t.Fatal("registered authority was deleted without detach lifecycle")
	}
}

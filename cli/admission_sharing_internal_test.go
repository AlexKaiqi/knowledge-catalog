package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	kcclient "kc/client"
	apphome "kc/home"
	"kc/identity"
	"kc/kernel"
)

func TestAdmissionAndSharingHaveTypedSelfServiceRoutes(t *testing.T) {
	dir := t.TempDir()
	f := &httpFacade{home: dir, readHome: &Home{Dir: dir}, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	f.registerServiceRoutes(mux)
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/identity/v1/admission", http.StatusOK},
		{"/catalog/v1/repositories/kr:%2F%2Fowner%2Fone/shares", http.StatusForbidden},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.Header.Set("X-Kc-As", "kaiqidong")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s: got %d %s; want %d", tc.path, w.Code, w.Body.String(), tc.want)
		}
	}
}

func admissionTestFacade(dir string, policy *apphome.AdmissionConfig, authenticator HTTPAuthenticator) http.Handler {
	f := &httpFacade{home: dir, readHome: &Home{Dir: dir, Deployment: &apphome.DeploymentConfig{Admission: policy}}, options: HTTPServerOptions{AuthMode: "local"}}
	if authenticator != nil {
		f.options = HTTPServerOptions{AuthMode: "gitea", Authenticator: authenticator}
	}
	mux := http.NewServeMux()
	f.registerAdmissionSharingRoutes(mux)
	return mux
}

func admissionHTTP(t *testing.T, h http.Handler, method, principal string) (*httptest.ResponseRecorder, kcclient.AdmissionResult) {
	t.Helper()
	r := httptest.NewRequest(method, "/identity/v1/admission", strings.NewReader(`{}`))
	r.Header.Set("X-Kc-As", principal)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var result kcclient.AdmissionResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w, result
}

func TestAdmissionRequiresExplicitHumanRequestAndNeverRegrantsAfterRestart(t *testing.T) {
	dir := t.TempDir()
	policy := &apphome.AdmissionConfig{Enabled: true, Catalog: "kr://platform/catalog", AuthenticatedUsers: true, Actions: []string{"catalog.read", "catalog.repositories.create"}}
	h := admissionTestFacade(dir, policy, nil)
	w, status := admissionHTTP(t, h, "GET", "kaiqidong")
	if w.Code != 200 || status.Status != "AVAILABLE" || !status.Eligible {
		t.Fatalf("first-use status: %d %s", w.Code, w.Body.String())
	}
	if PrincipalAllowed(dir, "kaiqidong", "catalog.read", "", policy.Catalog) {
		t.Fatal("viewing admission granted rights")
	}
	for _, machine := range []string{"agent:worker", "service:batch"} {
		w, _ = admissionHTTP(t, h, "POST", machine)
		if w.Code != 403 {
			t.Fatalf("machine admitted as human: %d %s", w.Code, w.Body.String())
		}
	}
	w, status = admissionHTTP(t, h, "POST", "kaiqidong")
	if w.Code != 200 || status.Status != "APPLIED" || !slices.Equal(status.CurrentActions, policy.Actions) {
		t.Fatalf("admission: %d %s", w.Code, w.Body.String())
	}
	if PrincipalAllowed(dir, "kaiqidong", "knowledge.read", "kr://private/repo", policy.Catalog) || PrincipalAllowed(dir, "kaiqidong", "catalog.read", "", "kr://other/catalog") {
		t.Fatal("admission widened its declared scope")
	}
	file, err := ReadAllow(dir)
	if err != nil {
		t.Fatal(err)
	}
	file.Rules = nil
	if err := WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	// An instance replacement and deployment policy expansion cannot restore
	// the revoked one-time policy or append newly configured actions.
	policy.Actions = append(policy.Actions, "catalog.repositories.connect")
	h = admissionTestFacade(dir, policy, nil)
	w, status = admissionHTTP(t, h, "POST", "kaiqidong")
	if w.Code != 200 || status.Status != "REPLAYED" || len(status.CurrentActions) != 0 || slices.Contains(status.Actions, "catalog.repositories.connect") {
		t.Fatalf("admission replay changed grant decision: %d %s", w.Code, w.Body.String())
	}
	file, _ = ReadAllow(dir)
	if len(file.Rules) != 0 || len(file.Admissions) != 1 {
		t.Fatalf("revocation lost: %#v", file)
	}
}

type admissionStaticAuth struct{ user HTTPIdentity }

func (admissionStaticAuth) Name() string { return "gitea" }

func (a admissionStaticAuth) Authenticate(context.Context, http.Header) (HTTPIdentity, error) {
	return a.user, nil
}

func TestAdmissionAuthenticatedUsersRequiresTrustedUserClaims(t *testing.T) {
	policy := &apphome.AdmissionConfig{Enabled: true, Catalog: "kr://platform/catalog", AuthenticatedUsers: true, Actions: []string{"catalog.read"}}
	for _, tc := range []struct {
		name string
		id   HTTPIdentity
		want int
	}{
		{"opaque principal", HTTPIdentity{Principal: "kaiqidong"}, 403},
		{"verified user", HTTPIdentity{Principal: "kaiqidong", User: &identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://idp.example", Subject: "42"}}, 200},
		{"delegated actor", HTTPIdentity{Principal: "agent:worker", OnBehalfOf: "kaiqidong", User: &identity.VerifiedUser{Username: "kaiqidong", Provider: "taihu", Issuer: "https://idp.example", Subject: "42"}}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := admissionTestFacade(t.TempDir(), policy, admissionStaticAuth{user: tc.id})
			w, _ := admissionHTTP(t, h, "POST", "")
			if w.Code != tc.want {
				t.Fatalf("got %d %s; want %d", w.Code, w.Body.String(), tc.want)
			}
		})
	}
}

func TestAdmissionConcurrentRequestsIssueOneDurablePolicy(t *testing.T) {
	dir := t.TempDir()
	h := admissionTestFacade(dir, &apphome.AdmissionConfig{Enabled: true, Catalog: "kr://platform/catalog", Principals: []string{"kaiqidong"}, Actions: []string{"catalog.read"}}, nil)
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			w, _ := admissionHTTP(t, h, "POST", "kaiqidong")
			if w.Code != 200 {
				t.Errorf("concurrent request: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	group.Wait()
	file, err := ReadAllow(dir)
	if err != nil || len(file.Rules) != 1 || len(file.Admissions) != 1 {
		t.Fatalf("concurrent issue duplicated: %#v %v", file, err)
	}
	w, _ := admissionHTTP(t, h, "POST", "someone")
	if w.Code != 403 {
		t.Fatalf("explicit principal policy admitted stranger: %d", w.Code)
	}
}

func TestRepositoryShareCannotWidenConsumptionAndRevokeIsScoped(t *testing.T) {
	dir := t.TempDir()
	repo, other := "kr://kaiqidong/one", "kr://someone/two"
	cx := &invocation{Home: dir, Flags: map[string]FlagValue{"as": "kaiqidong", "repo": repo}}
	policy := []string{"knowledge.read", "knowledge.search"}
	file := AllowFile{Version: allowVersion, Rules: []AllowRule{
		{ID: "owner", Principal: "kaiqidong", Repo: repo, Actions: []string{"repository.shares.manage", "knowledge.read"}},
		{ID: "limited", Principal: "kaiqidong", Repo: repo, Object: "public/*", Actions: []string{"knowledge.search"}},
		{ID: "someone-admin", Principal: "someone", Catalog: "kr://other/catalog", Actions: []string{"*"}},
	}}
	if err := WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"knowledge.search", "repository.shares.manage", "admin.grants.manage", "resource.access", "*"} {
		if _, err := createRepositoryShareWithPolicy(cx, kcclient.RepositoryShareRequest{Principal: "bob", Actions: []string{action}}, policy); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("widened or unmanaged action %s: %v", action, err)
		}
	}
	shared, err := createRepositoryShareWithPolicy(cx, kcclient.RepositoryShareRequest{Principal: "bob", Actions: []string{"knowledge.read"}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !PrincipalAllowed(dir, "bob", "knowledge.read", repo, "") || PrincipalAllowed(dir, "bob", "knowledge.read", other, "") || PrincipalAllowed(dir, "bob", "repository.shares.manage", repo, "") {
		t.Fatal("recipient rights are not limited to shared repo/action")
	}
	file, _ = ReadAllow(dir)
	file.Rules = append(file.Rules, AllowRule{ID: "other-share", ShareID: "other-share", SharedBy: "someone", Principal: "bob", Repo: other, Actions: []string{"knowledge.read"}})
	if err := WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"someone-admin", "other-share"} {
		cx.Flags["id"] = id
		if _, err := removeRepositoryShare(cx); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("removed unrelated rule %s: %v", id, err)
		}
	}
	cx.Flags["id"] = shared.ID
	if _, err := removeRepositoryShare(cx); err != nil {
		t.Fatal(err)
	}
	if PrincipalAllowed(dir, "bob", "knowledge.read", repo, "") || !PrincipalAllowed(dir, "bob", "knowledge.read", other, "") || !PrincipalAllowed(dir, "someone", "admin.grants.manage", "", "kr://other/catalog") {
		t.Fatal("share revocation touched unrelated rights or retained share")
	}
	file, _ = ReadAllow(dir)
	file.Rules = []AllowRule{{ID: "ref-limited", Principal: "kaiqidong", Repo: repo, Ref: "refs/heads/main", Actions: []string{"knowledge.read"}}}
	if err := WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	if _, err := createRepositoryShareWithPolicy(cx, kcclient.RepositoryShareRequest{Principal: "bob", Actions: []string{"knowledge.read"}}, policy); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("share widened ref-restricted rights: %v", err)
	}
}

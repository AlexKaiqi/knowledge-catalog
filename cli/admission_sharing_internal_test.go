package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kcclient "kc/client"
	apphome "kc/home"
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

func admissionTestFacade(dir string, policy *apphome.AdmissionConfig) http.Handler {
	f := &httpFacade{home: dir, readHome: &Home{Dir: dir, Deployment: &apphome.DeploymentConfig{Admission: policy}}, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	f.registerAdmissionSharingRoutes(mux)
	return mux
}

func TestAdmissionReportsOnlyCallerGrantsAndCurrentAdministrators(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAllow(dir, AllowFile{Version: allowVersion, Rules: []AllowRule{
		{ID: "mine", Principal: "kaiqidong", Repo: "kr://platform/one", Actions: []string{"knowledge.read"}},
		{ID: "other", Principal: "alice", Repo: "kr://platform/one", Actions: []string{"knowledge.read"}},
		{ID: "admin", Principal: "agent:operator", Actions: []string{"admin.grants.manage"}},
	}}); err != nil {
		t.Fatal(err)
	}
	h := admissionTestFacade(dir, &apphome.AdmissionConfig{RequestURL: "https://itsm.example/access"})
	r := httptest.NewRequest(http.MethodGet, "/identity/v1/admission", nil)
	r.Header.Set("X-Kc-As", "kaiqidong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("admission show: %d %s", w.Code, w.Body.String())
	}
	var result kcclient.AdmissionResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Principal != "kaiqidong" || len(result.Grants) != 1 || result.Grants[0].ID != "mine" {
		t.Fatalf("admission leaked another principal: %#v", result)
	}
	if result.Request.URL != "https://itsm.example/access" || len(result.Request.Administrators) != 1 || result.Request.Administrators[0] != "agent:operator" {
		t.Fatalf("request route is incomplete: %#v", result.Request)
	}
	post := httptest.NewRequest(http.MethodPost, "/identity/v1/admission", nil)
	post.Header.Set("X-Kc-As", "kaiqidong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST admission remains registered: %d", w.Code)
	}
}

func TestAdmissionListsBootstrapWildcardAsAdministrator(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAllow(dir, AllowFile{Version: allowVersion, Rules: []AllowRule{
		{ID: "bootstrap-deployment-admin", Principal: "kaiqidong", Actions: []string{"*"}},
	}}); err != nil {
		t.Fatal(err)
	}
	h := admissionTestFacade(dir, &apphome.AdmissionConfig{RequestURL: "https://itsm.example/access"})
	r := httptest.NewRequest(http.MethodGet, "/identity/v1/admission", nil)
	r.Header.Set("X-Kc-As", "reader")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("admission show: %d %s", w.Code, w.Body.String())
	}
	var result kcclient.AdmissionResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Principal != "reader" || len(result.Grants) != 0 {
		t.Fatalf("reader grants: %#v", result)
	}
	if len(result.Request.Administrators) != 1 || result.Request.Administrators[0] != "kaiqidong" {
		t.Fatalf("bootstrap * must appear as a grant manager: %#v", result.Request)
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

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apphome "kc/home"
	inv "kc/invocation"
	"kc/kernel"
)

func grantTestFacade(t *testing.T, rules []inv.AllowRule) (string, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	file := inv.AllowFile{Rules: rules, InitialGrants: map[string]kernel.Digest{"private-allocation": "secret-digest"}, Admissions: map[string]inv.AdmissionReceipt{"private-admission": {Principal: "reader"}}}
	if err := inv.WriteAllow(dir, file); err != nil {
		t.Fatal(err)
	}
	f := &httpFacade{home: dir, readHome: &Home{Dir: dir, File: HomeFile{Catalogs: []apphome.HomeCatalog{{ID: "catalog-A"}, {ID: "catalog-B"}}}}, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	f.registerManagementRoutes(mux)
	f.registerAdmissionSharingRoutes(mux)
	return dir, mux
}

func grantRequestForTest(t *testing.T, handler http.Handler, principal, method, target, body string, want int) map[string]any {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("X-Kc-As", principal)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: %d %s, want %d", method, target, w.Code, w.Body.String(), want)
	}
	var value map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGrantHTTPListFiltersWithoutEvaluationOrPrivateLedger(t *testing.T) {
	_, handler := grantTestFacade(t, []inv.AllowRule{
		{ID: "admin", Principal: "operator", Actions: []string{"*"}},
		{ID: "one", Principal: "reader", Catalog: "catalog-B", Dataset: "one", Actions: []string{"file.read"}},
		{ID: "two", Principal: "other", Catalog: "catalog-B", Dataset: "one", Actions: []string{"file.read"}},
		{ID: "three", Principal: "reader", Catalog: "catalog-A", Dataset: "one", Actions: []string{"file.read"}},
		{ID: "four", Principal: "reader", Repo: "repo-A", Ref: "main", Object: "data/*", Aspect: "body", Actions: []string{"knowledge.read"}},
	})
	for _, query := range []string{"?catalog=catalog-B&dataset=one&principal=reader&action=file.read", "?repository=repo-A&ref=main&object=data%2F%2A&aspect=body&principal=reader"} {
		value := grantRequestForTest(t, handler, "operator", http.MethodGet, "/admin/v1/grants"+query, "", http.StatusOK)
		if len(value) != 1 || value["rules"] == nil {
			t.Fatalf("list must contain only rules: %#v", value)
		}
		rules := value["rules"].([]any)
		if len(rules) != 1 {
			t.Fatalf("exact filters returned %d rules: %#v", len(rules), rules)
		}
	}
	value := grantRequestForTest(t, handler, "operator", http.MethodGet, "/admin/v1/grants?principal=missing&action=file.read", "", http.StatusOK)
	if len(value["rules"].([]any)) != 0 {
		t.Fatalf("empty filter evaluated instead of listing: %#v", value)
	}
}

func TestGrantAdministrationUsesExplicitAndDurableScopes(t *testing.T) {
	dir, handler := grantTestFacade(t, []inv.AllowRule{
		{ID: "read-A", Principal: "read-A", Catalog: "catalog-A", Actions: []string{"admin.grants.read"}},
		{ID: "manage-A", Principal: "manage-A", Catalog: "catalog-A", Actions: []string{"admin.grants.manage"}},
		{ID: "manage-B", Principal: "manage-B", Catalog: "catalog-B", Actions: []string{"admin.grants.manage"}},
		{ID: "read-repo", Principal: "read-repo", Repo: "repo-A", Actions: []string{"admin.grants.read"}},
		{ID: "manage-dataset", Principal: "manage-dataset", Catalog: "catalog-B", Dataset: "one", Actions: []string{"admin.grants.manage"}},
		{ID: "target-A", Principal: "reader", Catalog: "catalog-A", Actions: []string{"catalog.read"}},
		{ID: "target-B", Principal: "reader", Catalog: "catalog-B", Actions: []string{"catalog.read"}},
	})
	grantRequestForTest(t, handler, "read-A", http.MethodGet, "/admin/v1/grants", "", http.StatusForbidden)
	listed := grantRequestForTest(t, handler, "read-A", http.MethodGet, "/admin/v1/grants?catalog=catalog-A", "", http.StatusOK)
	for _, raw := range listed["rules"].([]any) {
		if raw.(map[string]any)["catalog"] != "catalog-A" {
			t.Fatalf("scoped reader saw another scope: %#v", listed)
		}
	}
	grantRequestForTest(t, handler, "read-A", http.MethodGet, "/admin/v1/grants?catalog=catalog-B", "", http.StatusForbidden)
	grantRequestForTest(t, handler, "read-repo", http.MethodGet, "/admin/v1/grants?repository=repo-A", "", http.StatusOK)
	grantRequestForTest(t, handler, "manage-A", http.MethodGet, "/admin/v1/grants?catalog=catalog-A", "", http.StatusForbidden)
	grantRequestForTest(t, handler, "manage-A", http.MethodPost, "/admin/v1/grants", `{"principal":"reader","actions":["knowledge.read"],"repository":"repo-B"}`, http.StatusForbidden)
	grantRequestForTest(t, handler, "manage-A", http.MethodDelete, "/admin/v1/grants/target-B?catalog=catalog-A", "", http.StatusForbidden)
	grantRequestForTest(t, handler, "manage-B", http.MethodDelete, "/admin/v1/grants/target-B", "", http.StatusOK)
	grantRequestForTest(t, handler, "manage-dataset", http.MethodPost, "/admin/v1/grants", `{"principal":"reader","actions":["file.read"],"catalog":"catalog-B","dataset":"one"}`, http.StatusOK)
	grantRequestForTest(t, handler, "manage-dataset", http.MethodPost, "/admin/v1/grants", `{"principal":"reader","actions":["file.read"],"catalog":"catalog-B","dataset":"two"}`, http.StatusForbidden)
	file, err := inv.ReadAllow(dir)
	if err != nil || len(file.InitialGrants) != 1 || len(file.Admissions) != 1 {
		t.Fatalf("management damaged durable receipts: %#v %v", file, err)
	}
}

func TestGrantAdministrationDoesNotInterpretTargetGlobsAsConcreteObjects(t *testing.T) {
	_, handler := grantTestFacade(t, []inv.AllowRule{{ID: "limited", Principal: "operator", Repo: "repo-A", Object: "data/?", Actions: []string{"admin.grants.manage"}}})
	grantRequestForTest(t, handler, "operator", http.MethodPost, "/admin/v1/grants", `{"principal":"reader","actions":["knowledge.read"],"repository":"repo-A","object":"data/*"}`, http.StatusForbidden)
}

func TestGrantHTTPRejectsUnknownOrRepeatedFilters(t *testing.T) {
	_, handler := grantTestFacade(t, []inv.AllowRule{{ID: "global", Principal: "operator", Actions: []string{"*"}}})
	for _, query := range []string{"?limit=1", "?principal=one&principal=two", "?catalog=%zz"} {
		grantRequestForTest(t, handler, "operator", http.MethodGet, "/admin/v1/grants"+query, "", http.StatusBadRequest)
	}
}

func TestGrantSelfRevocationDoesNotRestorePermissionAndReadDoesNotManage(t *testing.T) {
	_, handler := grantTestFacade(t, []inv.AllowRule{
		{ID: "bootstrap-deployment-admin", Principal: "operator", Actions: []string{"*"}},
		{ID: "read", Principal: "reader", Actions: []string{"admin.grants.read"}},
	})
	grantRequestForTest(t, handler, "reader", http.MethodDelete, "/admin/v1/grants/bootstrap-deployment-admin", "", http.StatusForbidden)
	grantRequestForTest(t, handler, "operator", http.MethodDelete, "/admin/v1/grants/bootstrap-deployment-admin", "", http.StatusOK)
	grantRequestForTest(t, handler, "operator", http.MethodGet, "/admin/v1/grants", "", http.StatusForbidden)
	grantRequestForTest(t, handler, "operator", http.MethodGet, "/identity/v1/admission", "", http.StatusOK)
}

func TestAdmissionIncludesEveryStoredScopeConstraint(t *testing.T) {
	_, handler := grantTestFacade(t, []inv.AllowRule{{ID: "limited", Principal: "reader", Catalog: "catalog-A", Dataset: "one", Ref: "main", Object: "data/*", Aspect: "body", Actions: []string{"file.read"}}})
	value := grantRequestForTest(t, handler, "reader", http.MethodGet, "/identity/v1/admission", "", http.StatusOK)
	grant := value["grants"].([]any)[0].(map[string]any)
	for name, want := range map[string]string{"dataset": "one", "ref": "main", "object": "data/*", "aspect": "body"} {
		if grant[name] != want {
			t.Errorf("admission lost %s: %#v", name, grant)
		}
	}
}

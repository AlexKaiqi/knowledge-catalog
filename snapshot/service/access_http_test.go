package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"kc/identity"
	"kc/snapshot/access"
)

type transportRuleStore struct {
	mu    sync.Mutex
	rules []access.Rule
}

func (s *transportRuleStore) ReadRules(context.Context) ([]access.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.rules), nil
}

func (s *transportRuleStore) UpdateRules(_ context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rules, err := update(slices.Clone(s.rules))
	if err == nil {
		s.rules = rules
	}
	return err
}

func transportAccessHandler(store *transportRuleStore) *handler {
	return newHandler(Config{
		Identity: IdentityOptions{Local: true},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
			return Resources{Access: access.NewService(store), RequestURL: "https://itsm.example/access"}, func() {}, nil
		},
	})
}

func transportCall(t *testing.T, handler http.Handler, principal, method, route, body string, want int) map[string]any {
	t.Helper()
	r := httptest.NewRequest(method, route, strings.NewReader(body))
	if principal != "" {
		r.Header.Set("X-Kc-As", principal)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: want %d got %d %s", method, route, want, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON response: %v %s", err, w.Body.String())
	}
	return out
}

func TestSnapshotAccessHTTPUsesVerifiedCallerAndExactDurableRevokeTarget(t *testing.T) {
	store := &transportRuleStore{rules: []access.Rule{
		{ID: "manager", Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "A"},
		{ID: "reader", Principal: "reader", Actions: []string{"admin.grants.read"}, Repo: "A"},
		{ID: "B-rule", Principal: "consumer", Actions: []string{"file.read"}, Repo: "B"},
	}}
	handler := transportAccessHandler(store)
	body := `{"principal":"consumer","actions":[" file.read, dataset.resolve "],"repository":"A","dataset":"published","ref":"main","object":"opaque/*","aspect":"canonical"}`
	transportCall(t, handler, "reader", http.MethodPost, "/admin/v1/grants", body, http.StatusForbidden)
	created := transportCall(t, handler, "manager", http.MethodPost, "/admin/v1/grants", body, http.StatusOK)
	id, ok := created["id"].(string)
	if !ok || id == "" || created["repo"] != "A" || created["dataset"] != "published" || created["object"] != "opaque/*" || !reflect.DeepEqual(created["actions"], []any{"file.read", "dataset.resolve"}) {
		t.Fatalf("grant receipt lost its original scope: %#v", created)
	}
	transportCall(t, handler, "manager", http.MethodGet, "/admin/v1/grants?repository=A", "", http.StatusForbidden)
	view := transportCall(t, handler, "reader", http.MethodGet, "/admin/v1/grants?repository=A&principal=consumer", "", http.StatusOK)
	rules := view["rules"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["id"] != id {
		t.Fatalf("inventory did not select exact rules: %#v", view)
	}
	for _, route := range []string{"/admin/v1/grants", "/admin/v1/grants?repository=B", "/admin/v1/grants?repository=source"} {
		transportCall(t, handler, "reader", http.MethodGet, route, "", http.StatusForbidden)
	}
	transportCall(t, handler, "manager", http.MethodDelete, "/admin/v1/grants/B-rule", "", http.StatusForbidden)
	transportCall(t, handler, "manager", http.MethodDelete, "/admin/v1/grants/"+id, `{"repository":"A"}`, http.StatusBadRequest)
	deleted := transportCall(t, handler, "manager", http.MethodDelete, "/admin/v1/grants/"+id, "", http.StatusOK)
	if deleted["revoked"] != id {
		t.Fatalf("revoke receipt changed: %#v", deleted)
	}
	transportCall(t, handler, "manager", http.MethodDelete, "/admin/v1/grants/"+id, "", http.StatusForbidden)
	transportCall(t, handler, "manager", http.MethodDelete, "/admin/v1/grants/manager", "", http.StatusOK)
	transportCall(t, handler, "manager", http.MethodPost, "/admin/v1/grants", body, http.StatusForbidden)
}

func TestSnapshotAdmissionIsPersonalAndNativeAdminDoesNotGrantAccess(t *testing.T) {
	store := &transportRuleStore{rules: []access.Rule{
		{ID: "own", Principal: "alice", Actions: []string{"writer.commit"}, Repo: "source", Dataset: "published", Ref: "main", Object: "opaque/*", Aspect: "canonical", SharedBy: "publisher"},
		{ID: "hidden", Principal: "bob", Actions: []string{"file.read"}, Repo: "secret"},
		{ID: "admin-z", Principal: "zed", Actions: []string{"admin.grants.manage"}, Repo: "A"},
		{ID: "admin-a", Principal: "amy", Actions: []string{"*"}},
	}}
	handler := transportAccessHandler(store)
	out := transportCall(t, handler, "alice", http.MethodGet, "/identity/v1/admission", "", http.StatusOK)
	grants := out["grants"].([]any)
	if len(grants) != 1 {
		t.Fatalf("another principal's grant leaked: %#v", out)
	}
	grant := grants[0].(map[string]any)
	for name, value := range map[string]any{"id": "own", "repository": "source", "dataset": "published", "ref": "main", "object": "opaque/*", "aspect": "canonical", "sharedBy": "publisher"} {
		if grant[name] != value {
			t.Fatalf("personal grant lost %s: %#v", name, grant)
		}
	}
	if _, exists := grant["repo"]; exists {
		t.Fatalf("admission used grant inventory's wire key: %#v", grant)
	}
	request := out["request"].(map[string]any)
	if request["url"] != "https://itsm.example/access" || !reflect.DeepEqual(request["administrators"], []any{"amy", "zed"}) {
		t.Fatalf("administrator contacts changed: %#v", request)
	}
	handler.config.Identity = IdentityOptions{Authenticator: &transportVerifier{mode: "lakefs", caller: identity.HTTPIdentity{Principal: "alice", Admin: true}}}
	r := httptest.NewRequest(http.MethodGet, "/admin/v1/grants", nil)
	r.Header.Set("Authorization", "Bearer native-admin-proof")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("native role silently became Snapshot access authority: %d %s", w.Code, w.Body.String())
	}
}

func TestSnapshotGrantFilterRejectsUnknownRepeatedAndMalformedQueries(t *testing.T) {
	handler := transportAccessHandler(&transportRuleStore{})
	for _, query := range []string{"unknown=value", "repository=A&repository=B", "repository=%zz", "repository=A;principal=reader", "repo=source"} {
		transportCall(t, handler, "reader", http.MethodGet, "/admin/v1/grants?"+query, "", http.StatusBadRequest)
	}
}

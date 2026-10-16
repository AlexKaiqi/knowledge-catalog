package cli

import (
	"context"
	"encoding/json"
	localauth "kc/identity/local"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"kc/identity"
	"kc/invoke"
	"kc/snapshot/access"
	snapshotservice "kc/snapshot/service"
)

type grantUpdatePolicyStore struct {
	mu    sync.Mutex
	rules []access.Rule
}

func (store *grantUpdatePolicyStore) ReadRules(context.Context) ([]access.Rule, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return slices.Clone(store.rules), nil
}

func (store *grantUpdatePolicyStore) UpdateRules(_ context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	next, err := update(slices.Clone(store.rules))
	if err == nil {
		store.rules = next
	}
	return err
}

func TestGrantUpdatePublicCLIUsesAtomicServiceScopesAndObservedRule(t *testing.T) {
	old := access.Rule{ID: "rule-A", Principal: "reader", Actions: []string{"file.read"}, Repo: "repo-A", Dataset: "published", Ref: "main", Object: "opaque/*", Aspect: "canonical", SharedBy: "publisher"}
	store := &grantUpdatePolicyStore{rules: []access.Rule{{ID: "manager", Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "repo-A"}, old}}
	service := snapshotservice.NewHandler(snapshotservice.Config{
		Identity: snapshotservice.IdentityOptions{Authenticator: localauth.New()},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (snapshotservice.Resources, func(), error) {
			return snapshotservice.Resources{Access: access.NewService(store)}, func() {}, nil
		},
	})
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		service.ServeHTTP(w, r)
	}))
	defer server.Close()
	call := func(principal, replacement string) RunResult {
		t.Helper()
		flags := grantUpdateFiles(t, replacement, observedGrantJSON)
		return Run([]string{"grant", "update", "--server", server.URL, "--as", principal,
			"--id", "rule-A", "--file", invoke.FlagString(flags, "file"), "--expected-file", invoke.FlagString(flags, "expected-file")})
	}
	replacement := `{"id":"rule-A","principal":"reader","actions":["file.read","dataset.resolve"],"repo":"repo-A","dataset":"published","sharedBy":"publisher"}`
	for name, attempt := range map[string]struct {
		principal string
		body      string
	}{
		"without-management": {"reader", replacement},
		"outside-new-scope":  {"manager", strings.Replace(replacement, `"repo-A"`, `"repo-B"`, 1)},
	} {
		t.Run(name, func(t *testing.T) {
			result := call(attempt.principal, attempt.body)
			current, _ := store.ReadRules(context.Background())
			if result.Status == 0 || !strings.Contains(result.Stdout, "FORBIDDEN") || !reflect.DeepEqual(current[1], old) {
				t.Fatalf("denied update changed policy or wrong failure: %#v %#v", result, current)
			}
		})
	}
	result := call("manager", replacement)
	if result.Status != 0 {
		t.Fatalf("grant update did not reach the existing atomic service: %s", result.Stdout)
	}
	var receipt access.Rule
	if err := json.Unmarshal([]byte(result.Stdout), &receipt); err != nil || receipt.ID != old.ID || receipt.SharedBy != old.SharedBy || receipt.Ref != "" || receipt.Object != "" || receipt.Aspect != "" || len(receipt.Actions) != 2 {
		t.Fatalf("full replacement lost identity/provenance or inherited omitted qualifiers: %#v %v", receipt, err)
	}
	result = call("manager", replacement)
	current, _ := store.ReadRules(context.Background())
	if result.Status == 0 || !strings.Contains(result.Stdout, "PRECONDITION_FAILED") || !reflect.DeepEqual(current[1], receipt) || len(current) != 2 {
		t.Fatalf("stale observation overwrote, duplicated or removed the rule: %#v %#v", result, current)
	}
	if !reflect.DeepEqual(methods, []string{http.MethodPut, http.MethodPut, http.MethodPut, http.MethodPut}) {
		t.Fatalf("CLI refreshed observation or decomposed an atomic update: %#v", methods)
	}
}

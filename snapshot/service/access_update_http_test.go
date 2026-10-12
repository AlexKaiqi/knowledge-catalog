package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"kc/snapshot/access"
)

func TestSnapshotGrantUpdateIsAtomicAndUsesOldAndNewScopes(t *testing.T) {
	old := access.Rule{ID: "target", Principal: "reader", Actions: []string{"file.read"}, Repo: "A", Dataset: "one", SharedBy: "publisher"}
	store := &transportRuleStore{rules: []access.Rule{{ID: "admin", Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "A"}, old}}
	h := transportAccessHandler(store)
	body := map[string]any{"principal": "reader", "actions": []string{"file.read", "dataset.resolve"}, "repository": "A", "dataset": "one", "expected": map[string]any{"id": old.ID, "principal": old.Principal, "actions": old.Actions, "repository": old.Repo, "dataset": old.Dataset, "sharedBy": old.SharedBy}}
	call := func(principal string, want int) map[string]any {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return transportCall(t, h, principal, http.MethodPut, "/admin/v1/grants/target", string(raw), want)
	}
	call("reader", http.StatusForbidden)
	body["repository"] = "B"
	call("manager", http.StatusForbidden)
	if len(store.rules) != 2 || len(store.rules[1].Actions) != 1 {
		t.Fatal("denied update changed policy")
	}
	body["repository"] = "A"
	updated := call("manager", http.StatusOK)
	if updated["id"] != "target" || updated["sharedBy"] != "publisher" || len(store.rules) != 2 || len(store.rules[1].Actions) != 2 {
		t.Fatalf("update changed identity/provenance: %#v", updated)
	}
	call("manager", http.StatusConflict)
	if len(store.rules[1].Actions) != 2 {
		t.Fatal("stale editor replaced a newer rule")
	}
}

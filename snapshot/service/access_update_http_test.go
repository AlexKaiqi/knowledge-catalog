package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"kc/snapshot/access"
)

func TestSnapshotGrantUpdateIsAtomicAndUsesOldAndNewScopes(t *testing.T) {
	old := access.Rule{ID: "target", Principal: "reader", Actions: []string{"file.read"}, Repo: "A", Dataset: "one", Ref: "main", Object: "opaque/*", Aspect: "canonical", SharedBy: "publisher"}
	store := &transportRuleStore{rules: []access.Rule{{ID: "admin", Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "A"}, old}}
	h := transportAccessHandler(store)
	body := map[string]any{"principal": "reader", "actions": []string{"file.read", "dataset.resolve"}, "repository": "A", "dataset": "one", "expected": map[string]any{"id": old.ID, "principal": old.Principal, "actions": old.Actions, "repository": old.Repo, "dataset": old.Dataset, "ref": old.Ref, "object": old.Object, "aspect": old.Aspect, "sharedBy": old.SharedBy}}
	call := func(principal string, want int) map[string]any {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return transportCall(t, h, principal, http.MethodPut, "/admin/v1/grants/target", string(raw), want)
	}
	snapshot := func() []byte {
		raw, err := json.Marshal(store.rules)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for _, test := range []struct{ name, principal, repository string }{{"read-only", "reader", "A"}, {"new-scope", "manager", "B"}} {
		t.Run(test.name, func(t *testing.T) {
			before := snapshot()
			body["repository"] = test.repository
			denied := call(test.principal, http.StatusForbidden)
			if denied["error"].(map[string]any)["code"] != "FORBIDDEN" || !bytes.Equal(snapshot(), before) {
				t.Fatalf("denied update changed policy or error code: %#v", denied)
			}
		})
	}
	body["repository"] = "A"
	updated := call("manager", http.StatusOK)
	var receipt access.Rule
	raw, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	want := access.Rule{ID: old.ID, Principal: old.Principal, Actions: []string{"file.read", "dataset.resolve"}, Repo: old.Repo, Dataset: old.Dataset, SharedBy: old.SharedBy}
	if err := json.Unmarshal(raw, &receipt); err != nil || !reflect.DeepEqual(receipt, want) || !reflect.DeepEqual(store.rules, []access.Rule{{ID: "admin", Principal: "manager", Actions: []string{"admin.grants.manage"}, Repo: "A"}, want}) {
		t.Fatalf("update changed identity/provenance: %#v", updated)
	}
	before := snapshot()
	conflict := call("manager", http.StatusConflict)
	if conflict["error"].(map[string]any)["code"] != "PRECONDITION_FAILED" || !bytes.Equal(snapshot(), before) {
		t.Fatalf("stale editor changed policy or error code: %#v", conflict)
	}
}

package access_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/snapshot/access"
)

func TestRulesPreserveWireQualifiersWithoutInterpretingFiles(t *testing.T) {
	var rule access.Rule
	raw := `{"id":"old","principal":"alice","actions":["dataset.consume","file.read","knowledge.read"],"repo":"source","catalog":"catalog","dataset":"published","ref":"main","object":"concept/*","aspect":"canonical","shareId":"share","sharedBy":"owner"}`
	if err := json.Unmarshal([]byte(raw), &rule); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rule.Actions, []string{"file.read", "knowledge.read"}) || rule.ShareID != "share" || rule.SharedBy != "owner" {
		t.Fatalf("legacy decode changed rule: %#v", rule)
	}
	query := access.Query{Principal: "alice", Action: "knowledge.read", Repo: "source", Catalog: "catalog", Dataset: "published", Ref: "main", Object: "concept/table", Aspect: "canonical"}
	if matched, ok := access.Match([]access.Rule{rule}, query); !ok || matched.ID != "old" {
		t.Fatalf("application qualifiers stopped matching: %#v %v", matched, ok)
	}
	query.Object = "files/table.json"
	if _, ok := access.Match([]access.Rule{rule}, query); ok {
		t.Fatal("a source file path replaced the application qualifier")
	}
	for _, invalid := range []string{`{"kset":"old"}`, `{"actions":["kset.read"]}`} {
		if err := json.Unmarshal([]byte(invalid), &rule); err == nil {
			t.Fatalf("retired wire representation was accepted: %s", invalid)
		}
	}
}

func TestInventoryIsExactAndCannotMutateTheAuthority(t *testing.T) {
	rules := []access.Rule{
		{ID: "exact", Principal: "alice", Actions: []string{"writer.*"}, Cmds: []string{"commit"}, Catalog: "A", Dataset: "published", Ref: "main", Object: "concept/*", Aspect: "canonical"},
		{ID: "other", Principal: "bob", Actions: []string{"writer.commit"}, Catalog: "A", Dataset: "published"},
	}
	filter := access.Query{Principal: "alice", Action: "writer.commit", Catalog: "A", Dataset: "published", Ref: "main", Object: "concept/*", Aspect: "canonical"}
	view := access.Inventory(rules, filter)
	if len(view.Rules) != 1 || view.Rules[0].ID != "exact" || view.Rules[0].Cmds != nil {
		t.Fatalf("inventory changed: %#v", view)
	}
	view.Rules[0].Actions[0] = "*"
	if rules[0].Actions[0] != "writer.*" || len(rules[0].Cmds) != 1 {
		t.Fatal("inventory mutated the durable rule")
	}
	filter.Object = "concept/table"
	if len(access.Inventory(rules, filter).Rules) != 0 {
		t.Fatal("inventory treated an exact selector as a glob query")
	}
}

func TestExplicitMatchDoesNotInferNamespacesOrNativeRoles(t *testing.T) {
	rules := []access.Rule{{Principal: "alice", Actions: []string{"file.read"}, Catalog: "A", Dataset: "published"}}
	for _, query := range []access.Query{
		{Principal: "alice", Action: "knowledge.search", Catalog: "A", Dataset: "published"},
		{Principal: "alice", Action: "file.read", Repo: "source"},
		{Principal: "native-admin", Action: "admin.grants.manage", Catalog: "A"},
		{Principal: "alice", Action: "file.read", Catalog: "B", Dataset: "published"},
	} {
		if _, ok := access.Match(rules, query); ok {
			t.Fatalf("explicit matcher inferred authority: %#v", query)
		}
	}
}

package access_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"kc/snapshot/access"
)

func TestRulesPreserveWireQualifiersWithoutInterpretingFiles(t *testing.T) {
	var rule access.Rule
	raw := `{"id":"old","principal":"alice","actions":["file.read","object.read"],"repo":"source","dataset":"published","ref":"main","object":"concept/*","aspect":"canonical","shareId":"share","sharedBy":"owner"}`
	if err := json.Unmarshal([]byte(raw), &rule); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rule.Actions, []string{"file.read", "object.read"}) || rule.ShareID != "share" || rule.SharedBy != "owner" {
		t.Fatalf("legacy decode changed rule: %#v", rule)
	}
	query := access.Query{Principal: "alice", Action: "object.read", Repo: "source", Dataset: "published", Ref: "main", Object: "concept/table", Aspect: "canonical"}
	if matched, ok := access.Match([]access.Rule{rule}, query); !ok || matched.ID != "old" {
		t.Fatalf("application qualifiers stopped matching: %#v %v", matched, ok)
	}
	query.Object = "files/table.json"
	if _, ok := access.Match([]access.Rule{rule}, query); ok {
		t.Fatal("a source file path replaced the application qualifier")
	}
	for _, invalid := range []string{`{"kset":"old"}`, `{"cmds":["read"]}`, `{"actions":["kset.read"]}`, `{"actions":["dataset.consume"]}`} {
		if err := json.Unmarshal([]byte(invalid), &rule); err == nil {
			t.Fatalf("retired wire representation was accepted: %s", invalid)
		}
	}
}

func TestInventoryIsExactAndCannotMutateTheAuthority(t *testing.T) {
	rules := []access.Rule{
		{ID: "exact", Principal: "alice", Actions: []string{"writer.*"}, Dataset: "published", Ref: "main", Object: "concept/*", Aspect: "canonical"},
		{ID: "other", Principal: "bob", Actions: []string{"writer.commit"}, Dataset: "published"},
	}
	filter := access.Query{Principal: "alice", Action: "writer.commit", Dataset: "published", Ref: "main", Object: "concept/*", Aspect: "canonical"}
	view := access.Inventory(rules, filter)
	if len(view.Rules) != 1 || view.Rules[0].ID != "exact" {
		t.Fatalf("inventory changed: %#v", view)
	}
	view.Rules[0].Actions[0] = "*"
	if rules[0].Actions[0] != "writer.*" {
		t.Fatal("inventory mutated the durable rule")
	}
	filter.Object = "concept/table"
	if len(access.Inventory(rules, filter).Rules) != 0 {
		t.Fatal("inventory treated an exact selector as a glob query")
	}
}

func TestExplicitMatchDoesNotInferNamespacesOrNativeRoles(t *testing.T) {
	rules := []access.Rule{{Principal: "alice", Actions: []string{"file.read"}, Dataset: "published"}}
	for _, query := range []access.Query{
		{Principal: "alice", Action: "object.search", Dataset: "published"},
		{Principal: "alice", Action: "file.read", Repo: "source"},
		{Principal: "native-admin", Action: "admin.grants.manage"},
		{Principal: "alice", Action: "file.read", Dataset: "other"},
	} {
		if _, ok := access.Match(rules, query); ok {
			t.Fatalf("explicit matcher inferred authority: %#v", query)
		}
	}
}

func TestExplicitMatchRequiresEveryCoordinateAndCombinedScope(t *testing.T) {
	rule := access.Rule{ID: "scoped", Principal: "alice", Actions: []string{"file.read"}, Repo: "source", Dataset: "published", Ref: "main", Object: "concept/*", Aspect: "canonical"}
	query := access.Query{Principal: "alice", Action: "file.read", Repo: "source", Dataset: "published", Ref: "main", Object: "concept/table", Aspect: "canonical"}
	for _, test := range []struct {
		name   string
		alter  func(*access.Rule, *access.Query)
		allows bool
	}{
		{name: "all-coordinates", allows: true},
		{name: "principal", alter: func(_ *access.Rule, q *access.Query) { q.Principal = "bob" }},
		{name: "action", alter: func(_ *access.Rule, q *access.Query) { q.Action = "dataset.resolve" }},
		{name: "repository-with-matching-dataset", alter: func(_ *access.Rule, q *access.Query) { q.Repo = "other" }},
		{name: "dataset-with-matching-repository", alter: func(_ *access.Rule, q *access.Query) { q.Dataset = "other" }},
		{name: "ref", alter: func(_ *access.Rule, q *access.Query) { q.Ref = "other" }},
		{name: "object", alter: func(_ *access.Rule, q *access.Query) { q.Object = "files/table" }},
		{name: "aspect", alter: func(_ *access.Rule, q *access.Query) { q.Aspect = "other" }},
		{name: "invalid-object-pattern", alter: func(r *access.Rule, _ *access.Query) { r.Object = "[" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, q := rule, query
			if test.alter != nil {
				test.alter(&r, &q)
			}
			matched, allowed := access.Match([]access.Rule{r}, q)
			if allowed != test.allows || allowed && matched.ID != rule.ID {
				t.Fatalf("coordinate evaluation: rule=%#v query=%#v matched=%#v allowed=%v", r, q, matched, allowed)
			}
		})
	}
}

package cli_test

import (
	"testing"

	"kc/internal/testkit"
)

// schema list is the summary layer: one Entity with several Aspects must come
// back as distinguishable rows from both entry points, so a caller can choose
// the Aspect it needs before any schema describe.
func TestSchemaListNamesEachAspectOfAnEntity(t *testing.T) {
	h := testkit.TempDir(t)
	repo := "kr://acme/summary/aspects"
	body(t, kc(h, "init"))
	seedRepo(t, h, repo)
	for _, schema := range []struct{ id, value string }{
		{"schema/service.health", `{"entity":"Service","aspect":"health","description":"Service health.","fields":{"status":{"type":"string"}}}`},
		{"schema/service.owner", `{"entity":"Service","aspect":"owner","description":"Who owns it.","fields":{"team":{"type":"string"},"oncall":{"type":"string"}}}`},
	} {
		body(t, kc(h, "put", testObjectReference(t, repo, schema.id, ""), "--repo", repo, "--command-id", schema.id, "--value", schema.value))
	}
	want := map[string][3]string{
		"schema/service.health": {"Service", "health", "Service health."},
		"schema/service.owner":  {"Service", "owner", "Who owns it."},
	}
	check := func(label string, page map[string]any, withCounts bool) {
		t.Helper()
		rows, _ := page["schemas"].([]any)
		seen := map[string]bool{}
		for _, raw := range rows {
			row := raw.(map[string]any)
			id, _ := row["objectId"].(string)
			expected, ok := want[id]
			if !ok {
				continue
			}
			seen[id] = true
			if row["entity"] != expected[0] || row["aspect"] != expected[1] || row["description"] != expected[2] {
				t.Fatalf("%s row must name entity, aspect and description: %#v", label, row)
			}
			if _, has := row["fields"]; has != withCounts {
				t.Fatalf("%s row fields presence = %v, want %v: %#v", label, has, withCounts, row)
			}
			if row["definition"] != nil {
				t.Fatalf("%s row must not carry the Schema contract: %#v", label, row)
			}
		}
		if len(seen) != len(want) {
			t.Fatalf("%s missed aspects: %#v", label, rows)
		}
	}
	check("repo list", asMap(t, body(t, kc(h, "schema", "list", "--repo", repo))), false)
	body(t, kc(h, "dataset", "define", "--dataset", "summary", "--revision", "1", "--source", repo+"=refs/heads/main"))
	check("dataset list", asMap(t, body(t, kc(h, "schema", "list", "--dataset", "summary"))), true)
}

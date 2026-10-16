package cli_test

import (
	"encoding/json"
	"testing"
)

// These are authored fixture declarations, chosen by each test. They do not
// infer defaults from Schema types or alter product publication behavior.
func seedTextDeclaration(t *testing.T, home, repository, declarationID, schema, field string) {
	t.Helper()
	seedIndexDeclaration(t, home, repository, declarationID, "text", map[string]any{
		"catalog": "declarative-index/opensearch@2026-10",
		"inputs":  []any{map[string]any{"from": schema + "#/fields/" + field}},
		"build":   map[string]any{"kind": "text"},
		"uses":    []any{"text"},
	})
}

func seedIndexDeclaration(t *testing.T, home, repository, declarationID, member string, value any, pathHint ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"writer", "put", "urn:kc:object:" + declarationID + "#/index/" + member, "--repo", repository, "--command-id", "declare-" + declarationID + "-" + member, "--kind", "Member", "--schema-ref", "schema/core/index-declaration/v1", "--value", string(raw)}
	if len(pathHint) > 0 {
		args = append(args, "--path-hint", pathHint[0])
	}
	body(t, kc(home, args...))
}

func datasetProjectionSnapshot(t *testing.T, report any, repository string) map[string]any {
	t.Helper()
	for _, member := range asMap(t, report)["members"].([]any) {
		row := asMap(t, member)
		if row["repository"] == repository {
			return asMap(t, asMap(t, report)["snapshot"])
		}
	}
	t.Fatalf("Dataset projection report omits %s: %#v", repository, report)
	return nil
}

func datasetProjectionState(t *testing.T, report any, repository string) map[string]any {
	t.Helper()
	for _, member := range asMap(t, report)["members"].([]any) {
		row := asMap(t, member)
		if row["repository"] == repository {
			return asMap(t, asMap(t, report)["state"])
		}
	}
	t.Fatalf("Dataset state projection report omits %s: %#v", repository, report)
	return nil
}

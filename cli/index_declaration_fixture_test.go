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

// datasetIndexMember finds the report row for one selected source. Both index
// commands answer with the fixed source vector, so a missing row means the
// report stopped describing the Dataset pin.
func datasetIndexMember(t *testing.T, report any, repository string) map[string]any {
	t.Helper()
	members, ok := asMap(t, report)["members"].([]any)
	if !ok {
		t.Fatalf("Dataset index report has no members: %#v", report)
	}
	for _, member := range members {
		row := asMap(t, member)
		if row["repository"] == repository {
			return row
		}
	}
	t.Fatalf("Dataset index report omits %s: %#v", repository, report)
	return nil
}

// datasetIndexBasis is the corpus a revision froze.
func datasetIndexBasis(t *testing.T, report any, repository string) map[string]any {
	t.Helper()
	datasetIndexMember(t, report, repository)
	basis, ok := asMap(t, report)["basis"].(map[string]any)
	if !ok {
		t.Fatalf("Dataset index report has no basis: %#v", report)
	}
	return basis
}

// datasetIndexObservation is the observation generation a prepare published.
func datasetIndexObservation(t *testing.T, report any, repository string) map[string]any {
	t.Helper()
	datasetIndexMember(t, report, repository)
	observation, ok := asMap(t, report)["observation"].(map[string]any)
	if !ok {
		t.Fatalf("Dataset index report has no observation: %#v", report)
	}
	return observation
}

// datasetIndexObservationIfPublished reads the observation only when the
// process published one. Observation publishing needs an injected runtime,
// which a test server without a service principal does not have, so the
// absence is a configuration fact rather than a contract failure.
func datasetIndexObservationIfPublished(t *testing.T, report any, repository string) (map[string]any, bool) {
	t.Helper()
	datasetIndexMember(t, report, repository)
	observation, ok := asMap(t, report)["observation"].(map[string]any)
	return observation, ok
}

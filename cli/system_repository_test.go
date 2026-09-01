package cli_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func TestLocalInitPublishesReadableImmutableSystemRepository(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/system-test"
	initialized := asMap(t, body(t, kc(home, "init", "--catalog", catalogID)))
	system := asMap(t, initialized["system"])
	if system["repositoryId"] != string(knowledge.SystemRepositoryID) || system["metaSchema"] != string(knowledge.MetaSchemaV1) {
		t.Fatalf("init did not publish System Repository: %#v", initialized)
	}

	state := asMap(t, body(t, kc(home, "read", "--catalog", catalogID)))
	if !containsString(state["repositories"].([]any), string(knowledge.SystemRepositoryID)) {
		t.Fatalf("System Repository is not registered: %#v", state)
	}

	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "user:admin"))
	report := asMap(t, body(t, kc(home, "describe-schema", "--as", "agent:any",
		"--repo", string(knowledge.SystemRepositoryID), "--object", string(knowledge.MetaSchemaV1))))
	if len(report["schemas"].([]any)) != 1 {
		t.Fatalf("public System Schema read failed: %#v", report)
	}
	expectCode(t, kc(home, "put", "--as", "user:admin", "--command-id", "mutate-system",
		"--repo", string(knowledge.SystemRepositoryID), "--object", "schema/evil",
		"--value", `{"entity":"Evil"}`), "FORBIDDEN")
}

func containsString(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

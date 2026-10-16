package cli_test

import (
	"testing"

	"kc/internal/testkit"
)

// TestMVPProviderConsumerJourney pins the two shortest role journeys described
// in doc/engineering/testing.md. It deliberately starts with a repository read:
// publishing object does not require a Workspace, while consuming it does.
func TestMVPProviderConsumerJourney(t *testing.T) {
	home := testkit.TempDir(t)
	repo := "kr://acme/public/core"
	body(t, kc(home, "init"))
	seedRepo(t, home, repo)
	body(t, kc(home, "put", testObjectReference(t, repo, "schema/runbook.body", ""), "--repo", repo, "--command-id", "schema-1", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string"}}}`))
	seedTextDeclaration(t, home, repo, "index/runbook-search", "schema/runbook.body", "body")
	receipt := asMap(t, asMap(t, body(t, kc(home, "put", testObjectReference(t, repo, "runbook/payment-oncall", ""), "--repo", repo, "--command-id", "source-1", "--schema-ref", "schema/runbook.body", "--value", `{"body":"切换支付流量前先检查冻结窗口"}`, "--origin-kind", "SOURCE", "--source-ref", "file:///source/runbooks/payment-oncall.md")))["result"])
	commit := receipt["newCommit"].(string)

	direct := asMap(t, body(t, kc(home, "read", testObjectReference(t, repo, "runbook/payment-oncall", ""), "--repo", repo)))
	if direct["commit"] != commit || asMap(t, direct["value"])["body"] != "切换支付流量前先检查冻结窗口" {
		t.Fatalf("provider could not read back the published value: %#v", direct)
	}

	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1",
		"--source", repo))
	state := asMap(t, body(t, kc(home, "show")))
	if len(state["datasets"].([]any)) != 1 {
		t.Fatalf("consumer could not discover the Workspace: %#v", state)
	}
	expectCode(t, kc(home, "workspace", "list"), "USAGE_INVALID")

	syncIndexes(t, home, "agent")
	search := asMap(t, body(t, kc(home, "search", "--dataset", "agent",
		"--query", "冻结窗口")))
	if search["completeness"] != "complete" || len(search["hits"].([]any)) != 1 {
		t.Fatalf("consumer search was not complete at the published dataset: %#v", search)
	}
	values := asMap(t, body(t, kc(home, "read", testObjectReference(t, repo, "runbook/payment-oncall", ""), "--dataset", "agent")))
	if values["reference"] != testObjectReference(t, repo, "runbook/payment-oncall", "") || asMap(t, values)["commit"] != commit {
		t.Fatalf("consumer read did not freeze the published commit: %#v", values)
	}
	provenance := body(t, kc(home, "provenance", "--dataset", "agent",
		"--object", "runbook/payment-oncall")).([]any)
	if len(provenance) != 1 || asMap(t, provenance[0])["commit"] != commit {
		t.Fatalf("consumer provenance did not freeze the published commit: %#v", provenance)
	}
}

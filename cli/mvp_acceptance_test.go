package cli_test

import (
	"encoding/json"
	"testing"

	"kc/internal/testkit"
)

// TestMVPProviderConsumerJourney pins the two shortest role journeys described
// in docs/MVP_ACCEPTANCE.md. It deliberately starts with a repository read:
// publishing knowledge does not require a Workspace, while consuming it does.
func TestMVPProviderConsumerJourney(t *testing.T) {
	home := testkit.TempDir(t)
	repo := "kr://acme/public/core"
	body(t, kc(home, "init", "--catalog", "acme/catalog"))
	seedRepo(t, home, repo)
	body(t, kc(home, "put", "--command-id", "schema-1", "--repo", repo,
		"--object", "schema/runbook.body",
		"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	receipt := asMap(t, asMap(t, body(t, kc(home, "put", "--command-id", "source-1", "--repo", repo,
		"--object", "runbook/payment-oncall", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"切换支付流量前先检查冻结窗口"}`,
		"--origin-kind", "SOURCE", "--source-ref", "file:///source/runbooks/payment-oncall.md")))["result"])
	commit := receipt["newCommit"].(string)

	direct := asMap(t, body(t, kc(home, "read", "--repo", repo,
		"--object", "runbook/payment-oncall")))
	if direct["commit"] != commit || asMap(t, direct["value"])["body"] != "切换支付流量前先检查冻结窗口" {
		t.Fatalf("provider could not read back the published value: %#v", direct)
	}

	body(t, kc(home, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", repo))
	state := asMap(t, body(t, kc(home, "read", "--catalog")))
	if len(state["workspaces"].([]any)) != 1 {
		t.Fatalf("consumer could not discover the Workspace: %#v", state)
	}
	workspaceList := asMap(t, body(t, kc(home, "workspace", "list")))
	if len(workspaceList["workspaces"].([]any)) != 1 {
		t.Fatalf("Catalog Workspace enumeration is bounded composition metadata: %#v", workspaceList)
	}
	pin := asMap(t, body(t, kc(home, "resolve", "--workspace", "agent")))
	pinJSON, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	if asMap(t, pin["repositories"])[repo] != commit {
		t.Fatalf("pin does not freeze the published commit: %#v", pin)
	}

	syncIndexes(t, home, repo)
	search := asMap(t, body(t, kc(home, "search", "--workspace", "agent", "--pin", string(pinJSON),
		"--query", "冻结窗口")))
	if search["completeness"] != "complete" || len(search["hits"].([]any)) != 1 {
		t.Fatalf("consumer search was not complete at the pin: %#v", search)
	}
	values := body(t, kc(home, "read", "--workspace", "agent", "--pin", string(pinJSON),
		"--object", "runbook/payment-oncall")).([]any)
	if len(values) != 1 || asMap(t, values[0])["commit"] != commit {
		t.Fatalf("consumer read did not reuse the pin: %#v", values)
	}
	provenance := body(t, kc(home, "provenance", "--workspace", "agent", "--pin", string(pinJSON),
		"--object", "runbook/payment-oncall")).([]any)
	if len(provenance) != 1 || asMap(t, provenance[0])["commit"] != commit {
		t.Fatalf("consumer provenance did not reuse the pin: %#v", provenance)
	}
}

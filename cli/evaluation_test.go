package cli_test

import (
	"fmt"
	"testing"

	"kc/internal/testkit"
)

func TestCLIEvaluationDailyJourneyProductStdout(t *testing.T) {
	home := testkit.TempDir(t)
	catalog := "kr://acme/catalog"
	repo := "kr://acme/public/core"
	body(t, kc(home, "init", "--catalog", catalog))
	seedRepo(t, home, repo)
	body(t, kc(home, "put", "--command-id", "schema-1", "--repo", repo,
		"--object", "schema/runbook.body",
		"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(home, "put", "--command-id", "source-1", "--repo", repo,
		"--object", "runbook/payment", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"切换支付流量前先检查冻结窗口"}`))
	body(t, kc(home, "grant", "add", "--principal", "bot", "--action", "catalog.read", "--catalog", catalog))
	body(t, kc(home, "grant", "add", "--principal", "bot", "--action", "knowledge.read", "--repo", repo))

	read := asMap(t, body(t, kc(home, "read", "--repo", repo, "--object", "runbook/payment")))
	if read["objectId"] != "runbook/payment" || read["repository"] != repo {
		t.Fatalf("read identity: %#v", read)
	}
	if asMap(t, read["value"])["body"] != "切换支付流量前先检查冻结窗口" {
		t.Fatalf("read body: %#v", read)
	}
	if _, ok := read["knowledgeRef"]; ok {
		t.Fatalf("read kept protocol envelope: %#v", read)
	}
	if _, ok := read["address"]; ok {
		t.Fatalf("read kept address envelope: %#v", read)
	}

	describe := asMap(t, body(t, kc(home, "schema", "describe", "--repo", repo, "--object", "schema/runbook.body")))
	if describe["repository"] != repo {
		t.Fatalf("describe repository: %#v", describe)
	}
	schemas, _ := describe["schemas"].([]any)
	if len(schemas) != 1 {
		t.Fatalf("describe schemas: %#v", describe)
	}
	schema := asMap(t, schemas[0])
	if schema["objectId"] != "schema/runbook.body" {
		t.Fatalf("describe objectId: %#v", describe)
	}
	if _, ok := schema["entity"]; ok {
		t.Fatalf("describe kept list-like entity: %#v", describe)
	}
	fields, _ := schema["fields"].([]any)
	if len(fields) == 0 {
		t.Fatalf("describe dropped fields: %#v", describe)
	}

	resolved := asMap(t, body(t, kc(home, "resolve", "--repo", repo, "--object", "runbook/payment")))
	if resolved["status"] != "RESOLVED" || resolved["objectId"] != "runbook/payment" {
		t.Fatalf("resolve identity: %#v", resolved)
	}
	if _, ok := resolved["address"]; ok {
		t.Fatalf("resolve kept address envelope: %#v", resolved)
	}

	listed := asMap(t, body(t, kc(home, "grant", "list", "--repo", repo)))
	if _, ok := listed["version"]; ok {
		t.Fatalf("grant list dumped allow file: %#v", listed)
	}
	if _, ok := listed["initialGrants"]; ok {
		t.Fatalf("grant list dumped initialGrants: %#v", listed)
	}
	rules, _ := listed["rules"].([]any)
	if len(rules) == 0 {
		t.Fatalf("grant list --repo dropped repository rules: %#v", listed)
	}
	for _, raw := range rules {
		rule := asMap(t, raw)
		if fmt.Sprint(rule["catalog"]) != "" && fmt.Sprint(rule["catalog"]) != "<nil>" {
			t.Fatalf("grant list --repo included catalog rule: %#v", listed)
		}
		if fmt.Sprint(rule["repo"]) != repo {
			t.Fatalf("grant list --repo included other repo: %#v", listed)
		}
	}
}

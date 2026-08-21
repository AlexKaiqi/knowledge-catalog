package cli_test

import (
	"testing"

	"kc/internal/testkit"
)

func TestConsumeViewFollowsPublishedBranch(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	c1 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "v1",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"body":"needs a runbook"}`,
	)))["result"])["newCommit"].(string)
	body(t, kc(h, "define-view", "--view", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	c2 := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A", "--value", `{"body":"later live"}`)))["result"])["newCommit"].(string)

	serving := body(t, kc(h, "read", "--view", "agent", "--object", "policy/A")).([]any)
	if len(serving) != 1 {
		t.Fatal(serving)
	}
	got := asMap(t, asMap(t, serving[0])["value"])
	if got["body"] != "later live" {
		t.Fatal("consumer read follows the published branch", got)
	}
	if asMap(t, serving[0])["commit"] != c2 {
		t.Fatal("result still names the resolved commit; caller did not pass it", serving[0], c1, c2)
	}

	space := asMap(t, body(t, kc(h, "read", "--catalog")))
	if space["catalogId"] != "kr://acme/catalog" {
		t.Fatal(space)
	}
	if _, ok := space["releases"]; ok {
		t.Fatal("read --catalog must not list releases", space)
	}
	if _, ok := space["generations"]; ok {
		t.Fatal("read --catalog must not list generations", space)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--catalog"), "FORBIDDEN")

	listed := body(t, kc(h, "list", "--view", "agent")).([]any)
	sawA := false
	for _, item := range listed {
		if asMap(t, item)["objectId"] == "policy/A" {
			sawA = true
			if asMap(t, asMap(t, item)["value"])["body"] != "later live" {
				t.Fatal("list follows published branch", item)
			}
		}
	}
	if !sawA {
		t.Fatal(listed)
	}

	hits := body(t, kc(h, "search", "--view", "agent", "--query", "later")).([]any)
	if len(hits) != 1 {
		t.Fatalf("search --view: %#v", hits)
	}
	desc := asMap(t, body(t, kc(h, "describe-index", "--repo", core)))
	if desc["basisCommit"] != c2 {
		t.Fatalf("live index follows HEAD: %#v", desc)
	}

	logs := body(t, kc(h, "log", "--view", "agent", "--object", "policy/A")).([]any)
	if len(logs) != 1 {
		t.Fatalf("log --view: %#v", logs)
	}
	log0 := asMap(t, logs[0])
	if log0["commit"] != c2 {
		t.Fatalf("object log must name the resolved commit: %#v", log0)
	}
	hist := asMap(t, body(t, kc(h, "audit", "--view", "agent")))
	if hist["source"] != "catalog" {
		t.Fatal("view-filtered registry history is audit", hist)
	}
	expectMsg(t, kc(h, "log", "--catalog"), "kc audit")
	expectMsg(t, kc(h, "log", "--view", "agent"), "missing --object")
	expectMsg(t, kc(h, "log", "--view", "agent", "--repo", core, "--object", "policy/A"), "cannot be combined")

	resolved := body(t, kc(h, "resolve", "--view", "agent", "--object", "policy/A")).([]any)
	if len(resolved) != 1 || asMap(t, resolved[0])["status"] != "RESOLVED" {
		t.Fatal(resolved)
	}

	live := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["body"] != "later live" {
		t.Fatal("maintainer read --repo still follows the named ref", live)
	}

	expectCode(t, kc(h, "read", "--as", "bot", "--view", "agent", "--object", "policy/A"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-view", "--catalog", "kr://acme/catalog", "--view", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", core))
	asBot := body(t, kc(h, "read", "--as", "bot", "--view", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, asBot[0])["value"])["body"] != "later live" {
		t.Fatal(asBot)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--catalog"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-catalog", "--catalog", "kr://acme/catalog"))
	asBotSpace := asMap(t, body(t, kc(h, "read", "--as", "bot", "--catalog")))
	if asBotSpace["catalogId"] != "kr://acme/catalog" {
		t.Fatal(asBotSpace)
	}

	expectMsg(t, kc(h, "read", "--view", "agent", "--repo", core, "--object", "policy/A"), "cannot be combined")
	expectMsg(t, kc(h, "read", "--view", "agent", "--commit", c1, "--object", "policy/A"), "cannot be combined")
	expectMsg(t, kc(h, "list", "--view", "agent", "--ref", "refs/heads/main"), "cannot be combined")
	expectCode(t, kc(h, "read", "--view", "missing", "--object", "policy/A"), "VIEW_GENERATION_INVALID")
	expectMsg(t, kc(h, "promote", "--release", "stable", "--view", "agent"), "unknown command promote")
	expectMsg(t, kc(h, "read", "--release", "stable", "--object", "policy/A"), "unknown flag --release")
}

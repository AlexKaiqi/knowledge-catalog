package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
)

func TestConsumeViewFollowsPublishedBranch(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "put", "--command-id", "task-io", "--repo", core,
		"--object", "ETLTask:daily-orders", "--aspect", "io", "--value", `{"inputs":["orders"]}`))
	body(t, kc(h, "put", "--command-id", "schema-body", "--repo", core, "--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"access":["text"]}}}`))
	c1 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "v1",
		"--repo", core,
		"--object", "policy/A",
		"--value", `{"body":"needs a runbook"}`,
	)))["result"])["newCommit"].(string)
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	c2 := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A", "--value", `{"body":"later live 冻结窗口"}`)))["result"])["newCommit"].(string)

	serving := body(t, kc(h, "read", "--workspace", "agent", "--object", "policy/A")).([]any)
	if len(serving) != 1 {
		t.Fatal(serving)
	}
	got := asMap(t, asMap(t, serving[0])["value"])
	if got["body"] != "later live 冻结窗口" {
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

	listed := body(t, kc(h, "list", "--workspace", "agent")).([]any)
	sawA := false
	for _, item := range listed {
		if asMap(t, item)["objectId"] == "policy/A" {
			sawA = true
			if asMap(t, asMap(t, item)["value"])["body"] != "later live 冻结窗口" {
				t.Fatal("list follows published branch", item)
			}
		}
	}
	if !sawA {
		t.Fatal(listed)
	}

	search := asMap(t, body(t, kc(h, "search", "--workspace", "agent", "--query", "later")))
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("search --workspace: %#v", hits)
	}
	if search["completeness"] != "complete" {
		t.Fatalf("exact workspace search must be complete: %#v", search)
	}
	cjkSearch := asMap(t, body(t, kc(h, "search", "--workspace", "agent", "--query", "冻结窗口")))
	if cjkSearch["completeness"] != "complete" || len(cjkSearch["hits"].([]any)) != 1 {
		t.Fatalf("declared text search must support contiguous CJK text: %#v", cjkSearch)
	}
	pin := asMap(t, body(t, kc(h, "resolve", "--workspace", "agent")))
	if pin["workspaceId"] != "agent" {
		t.Fatalf("resolve --workspace pin: %#v", pin)
	}
	if _, legacy := pin["appendCuts"]; legacy {
		t.Fatalf("Snapshot pin must not carry dynamic observation cuts: %#v", pin)
	}
	if asMap(t, pin["repositories"])[core] != c2 {
		t.Fatalf("pin must name this command's commits: %#v", pin)
	}
	hit0 := asMap(t, asMap(t, hits[0])["knowledge"])
	if asMap(t, hit0["knowledgeRef"])["object"] != "policy/A" {
		t.Fatalf("search envelope: %#v", hit0)
	}
	read0 := asMap(t, serving[0])
	if asMap(t, read0["knowledgeRef"])["object"] != "policy/A" || asMap(t, read0["address"])["objectId"] != "policy/A" {
		t.Fatalf("read --workspace must share KnowledgeValue fields: %#v", read0)
	}
	ins := asMap(t, body(t, kc(h, "inspect", "--workspace", "agent")))
	if asMap(t, ins["pin"])["workspaceId"] != "agent" {
		t.Fatalf("inspect pin: %#v", ins)
	}
	if asMap(t, ins["catalog"])["catalogId"] == "" {
		t.Fatalf("inspect catalog: %#v", ins["catalog"])
	}
	if len(ins["indexes"].([]any)) != 1 {
		t.Fatalf("inspect indexes: %#v", ins["indexes"])
	}
	insIdx := asMap(t, ins["indexes"].([]any)[0])
	if insIdx["basisCommit"] != c2 || insIdx["lagBehindHead"] != false {
		t.Fatalf("inspect must describe this Workspace pin, not a stale live index: %#v pin %s", insIdx, c2)
	}
	desc := asMap(t, body(t, kc(h, "describe-index", "--repo", core)))
	if desc["basisCommit"] != c2 {
		t.Fatalf("live index follows HEAD: %#v", desc)
	}

	logs := body(t, kc(h, "log", "--workspace", "agent", "--object", "policy/A")).([]any)
	if len(logs) != 1 {
		t.Fatalf("log --workspace: %#v", logs)
	}
	log0 := asMap(t, logs[0])
	if log0["commit"] != c2 {
		t.Fatalf("object log must name the resolved commit: %#v", log0)
	}
	hist := asMap(t, body(t, kc(h, "audit", "--workspace", "agent")))
	if hist["source"] != "catalog" {
		t.Fatal("workspace-filtered registry history is audit", hist)
	}
	expectMsg(t, kc(h, "log", "--catalog"), "kc audit")
	expectMsg(t, kc(h, "log", "--workspace", "agent"), "missing --object")
	expectMsg(t, kc(h, "log", "--workspace", "agent", "--repo", core, "--object", "policy/A"), "cannot be combined")

	resolved := body(t, kc(h, "resolve", "--workspace", "agent", "--object", "policy/A")).([]any)
	if len(resolved) != 1 || asMap(t, resolved[0])["status"] != "RESOLVED" {
		t.Fatal(resolved)
	}
	resolvedAspect := body(t, kc(h, "resolve", "--workspace", "agent", "--object", "ETLTask:daily-orders", "--aspect", "io")).([]any)
	if len(resolvedAspect) != 1 || asMap(t, resolvedAspect[0])["status"] != "RESOLVED" || asMap(t, asMap(t, resolvedAspect[0])["address"])["aspectName"] != "io" || asMap(t, resolvedAspect[0])["digest"] == "" {
		t.Fatal("resolve --workspace --aspect must return the exact unit Address and digest", resolvedAspect)
	}

	live := asMap(t, body(t, kc(h, "read", "--repo", core, "--object", "policy/A", "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["body"] != "later live 冻结窗口" {
		t.Fatal("maintainer read --repo still follows the named ref", live)
	}

	expectCode(t, kc(h, "read", "--as", "bot", "--workspace", "agent", "--object", "policy/A"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", core))
	asBot := body(t, kc(h, "read", "--as", "bot", "--workspace", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, asBot[0])["value"])["body"] != "later live 冻结窗口" {
		t.Fatal(asBot)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--catalog"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-catalog", "--catalog", "kr://acme/catalog"))
	asBotSpace := asMap(t, body(t, kc(h, "read", "--as", "bot", "--catalog")))
	if asBotSpace["catalogId"] != "kr://acme/catalog" {
		t.Fatal(asBotSpace)
	}

	expectMsg(t, kc(h, "read", "--workspace", "agent", "--repo", core, "--object", "policy/A"), "cannot be combined")
	expectMsg(t, kc(h, "read", "--workspace", "agent", "--commit", c1, "--object", "policy/A"), "cannot be combined")
	expectMsg(t, kc(h, "list", "--workspace", "agent", "--ref", "refs/heads/main"), "cannot be combined")
	expectCode(t, kc(h, "read", "--workspace", "missing", "--object", "policy/A"), "WORKSPACE_INVALID")
	expectMsg(t, kc(h, "promote", "--workspace", "agent"), "unknown command promote")
	expectMsg(t, kc(h, "read", "--release", "stable", "--object", "policy/A"), "unknown flag --release")
}

func TestWorkspaceAuthorizationCoverageIsHonest(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	public := "kr://acme/public/runbooks"
	private := "kr://acme/private/runbooks"
	body(t, kc(h, "init", "--catalog", catalogID))
	for _, repo := range []string{public, private} {
		body(t, kc(h, "repo-add", "--repo", repo))
		body(t, kc(h, "put", "--command-id", "schema-"+repo, "--repo", repo,
			"--object", "schema/runbook.body",
			"--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	}
	body(t, kc(h, "put", "--command-id", "public-body", "--repo", public,
		"--object", "runbook/public", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment public procedure"}`))
	body(t, kc(h, "put", "--command-id", "private-body", "--repo", private,
		"--object", "runbook/private", "--schema-ref", "schema/runbook.body",
		"--value", `{"body":"payment private procedure"}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1",
		"--source", public+"=refs/heads/main", "--source", private+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace",
		"--catalog", catalogID, "--workspace", "agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", public))

	// Bare-array reads cannot honestly represent partial coverage, so they fail
	// closed instead of making a hidden member look like an absent object.
	expectCode(t, kc(h, "read", "--as", "bot", "--workspace", "agent",
		"--object", "runbook/private"), "FORBIDDEN")
	expectCode(t, kc(h, "relations", "--as", "bot", "--workspace", "agent",
		"--object", "runbook/public"), "FORBIDDEN")
	expectCode(t, kc(h, "resolve", "--as", "bot", "--workspace", "agent"), "FORBIDDEN")
	expectCode(t, kc(h, "inspect", "--as", "bot", "--workspace", "agent"), "FORBIDDEN")
	expectCode(t, kc(h, "describe-access", "--as", "bot", "--workspace", "agent"), "FORBIDDEN")

	// SEARCH has a coverage envelope, so it may serve the authorized subset but
	// must not expose the hidden member or call that subset complete.
	search := asMap(t, body(t, kc(h, "search", "--as", "bot", "--workspace", "agent", "--query", "payment")))
	if search["completeness"] != "partial" {
		t.Fatalf("authorization clipping must be partial: %#v", search)
	}
	claims := search["claims"].([]any)
	if len(claims) == 0 || claims[0] != "some workspace members were omitted by authorization" {
		t.Fatalf("authorization clipping needs a non-sensitive claim: %#v", search)
	}
	snapshots := asMap(t, asMap(t, search["view"])["snapshots"])
	if len(snapshots) != 1 || snapshots[public] == nil || snapshots[private] != nil {
		t.Fatalf("search view exposed a hidden member: %#v", search)
	}
	hits := search["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("authorized search subset: %#v", search)
	}
	knowledge := asMap(t, asMap(t, hits[0])["knowledge"])
	if knowledge["repository"] != public {
		t.Fatalf("unauthorized hit escaped filtering: %#v", search)
	}
}

func TestCheckoutViewPin(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	group := "kr://acme/groups/payments"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "repo-add", "--repo", group))
	body(t, kc(h, "put",
		"--command-id", "pub",
		"--repo", core,
		"--object", "policy/P-103",
		"--value", `{"body":"public"}`,
	))
	body(t, kc(h, "put", "--command-id", "runbook", "--repo", core, "--object", "runbooks/oncall", "--value", `{"text":"freeze"}`))
	body(t, kc(h, "put", "--command-id", "grp", "--repo", group, "--object", "policy/P-103", "--value", `{"body":"group"}`))
	body(t, kc(h, "define-workspace", "--workspace", "payments-agent", "--revision", "1",
		"--source", core+"=refs/heads/main",
		"--source", group+"=refs/heads/main"))

	out := asMap(t, body(t, kc(h, "checkout", "--workspace", "payments-agent")))
	if out["workspaceId"] != "payments-agent" || out["dir"] != "checkouts/payments-agent" {
		t.Fatalf("%#v", out)
	}
	if int(out["objects"].(float64)) < 2 {
		t.Fatalf("objects %#v", out["objects"])
	}
	pin := asMap(t, out["pin"])
	if pin["provider"] != "grep" || pin["workspaceId"] != "payments-agent" {
		t.Fatalf("pin %#v", pin)
	}
	repos := asMap(t, pin["repositories"])
	if repos[core] == nil || repos[core] == "" {
		t.Fatalf("pin must name resolved commits: %#v", pin)
	}

	root := filepath.Join(h, "checkouts", "payments-agent")
	publicFile := filepath.Join(root, "kr_acme_public_core", "policy", "P-103.json")
	groupFile := filepath.Join(root, "kr_acme_groups_payments", "policy", "P-103.json")
	if checkoutJSON(t, publicFile)["body"] != "public" || checkoutJSON(t, groupFile)["body"] != "group" {
		t.Fatal("same object_id stays two files")
	}
	if checkoutJSON(t, filepath.Join(root, "kr_acme_public_core", "runbooks", "oncall.json"))["text"] != "freeze" {
		t.Fatal("path is object_id")
	}
	info, err := os.Stat(publicFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("read-only: %s", info.Mode())
	}

	c2 := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "later",
		"--repo", core,
		"--object", "policy/P-103",
		"--value", `{"body":"later live"}`,
	)))["result"])["newCommit"].(string)
	if checkoutJSON(t, publicFile)["body"] != "public" {
		t.Fatal("checkout must not follow HEAD until re-run")
	}
	again := asMap(t, body(t, kc(h, "checkout", "--workspace", "payments-agent")))
	if checkoutJSON(t, publicFile)["body"] != "later live" {
		t.Fatal("re-checkout follows the published branch")
	}
	if asMap(t, again["pin"])["repositories"].(map[string]any)[core] != c2 {
		t.Fatalf("pin must move: %#v", again["pin"])
	}

	expectMsg(t, kc(h, "checkout", "--workspace", "payments-agent", "--repo", core), "cannot be combined")
	expectMsg(t, kc(h, "checkout"), "requires --workspace")
	expectCode(t, kc(h, "checkout", "--as", "bot", "--workspace", "payments-agent"), "FORBIDDEN")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "payments-agent"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", core))
	asBot := asMap(t, body(t, kc(h, "checkout", "--as", "bot", "--workspace", "payments-agent")))
	if int(asBot["objects"].(float64)) < 1 {
		t.Fatalf("bot checkout %#v", asBot)
	}
	if _, err := os.Stat(groupFile); !os.IsNotExist(err) {
		t.Fatal("bot without group read must not materialize that repo")
	}
	if checkoutJSON(t, publicFile)["body"] != "later live" {
		t.Fatal("bot still sees allowed repo")
	}
}

func checkoutJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err, string(raw))
	}
	return out
}

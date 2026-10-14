package cli_test

import (
	"testing"

	"kc/internal/testkit"
)

// TestUserJourneyAttachExistingRepository proves authority attachment as a
// user operation. The existing lakeFS authority is opened over its DSN and
// registered in the Catalog; opening may bootstrap the Knowledge tables.
func TestUserJourneyAttachExistingRepository(t *testing.T) {
	h := testkit.TempDir(t)
	sourceHome := testkit.TempDir(t)
	repoID := "kr://acme/teams/platform"
	authorityDSN := lakeFSRepoDSN(t)
	body(t, kc(sourceHome, "init"))
	seedRepo(t, sourceHome, repoID, "--dsn", authorityDSN)
	body(t, kc(h, "init"))
	seedRepo(t, h, repoID, "--dsn", authorityDSN)
	state := asMap(t, body(t, kc(h, "show")))
	repositories := businessRepositories(state)
	if len(repositories) != 1 || repositories[0] != repoID {
		t.Fatalf("attached repository is not registered: %#v", state)
	}
}

// TestUserJourneyManageAgentAccess covers the whole access-management job as
// an owner experiences it: grant a Workspace and its member, inspect the
// decision, consume as the agent, revoke the Workspace grant, and observe the
// denial immediately. --dataset is intentional: it is the primary product
// vocabulary; Workspace is the only composition and permission scope.
func TestUserJourneyManageAgentAccess(t *testing.T) {
	h := testkit.TempDir(t)

	repoID := "kr://acme/public/runbooks"
	setID := "oncall-agent"
	body(t, kc(h, "init"))
	seedRepo(t, h, repoID)
	body(t, kc(h, "put", testObjectReference(t, repoID, "runbook/payments", ""), "--command-id", "seed", "--value", `{"text":"freeze traffic"}`))
	body(t, kc(h, "dataset", "define", "--dataset", setID, "--revision", "1",
		"--source", repoID+"=refs/heads/main"))

	workspaceRule := asMap(t, body(t, kc(h, "allow", "--principal", "agent",
		"--cmd", "read-workspace", "--dataset", setID)))
	if workspaceRule["dataset"] != setID {
		t.Fatalf("--dataset scope was not stored on the rule: %#v", workspaceRule)
	}
	repoRule := asMap(t, body(t, kc(h, "allow", "--principal", "agent",
		"--cmd", "read", "--repo", repoID)))

	who := asMap(t, body(t, kc(h, "whoami", "--as", "agent")))
	if who["principal"] != "agent" {
		t.Fatal(who)
	}
	values := asMap(t, body(t, kc(h, "read", testObjectReference(t, repoID, "runbook/payments", ""), "--as", "agent", "--dataset", setID)))
	if values["reference"] != testObjectReference(t, repoID, "runbook/payments", "") || asMap(t, asMap(t, values)["value"])["text"] != "freeze traffic" {
		t.Fatal(values)
	}
	rules := asMap(t, body(t, kc(h, "allowed")))
	if rows := rules["rules"].([]any); len(rows) != 2 {
		t.Fatalf("both grants must be stored: %#v", rules)
	}

	body(t, kc(h, "revoke", "--id", workspaceRule["id"].(string)))
	expectCode(t, kc(h, "read", testObjectReference(t, repoID, "runbook/payments", ""), "--as", "agent", "--dataset", setID), "FORBIDDEN")

	// The repository grant is independent and remains until it is explicitly
	// revoked; revoking the Workspace grant must not silently delete it.
	rules = asMap(t, body(t, kc(h, "allowed")))
	rows := rules["rules"].([]any)
	if len(rows) != 1 || asMap(t, rows[0])["id"] != repoRule["id"] {
		t.Fatalf("unexpected rules after revoke: %#v", rules)
	}
}

// TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess separates two things a
// user can easily confuse: a permissions Aspect describes source-system
// knowledge, while kc allow controls who may consume a Repository/Workspace.
func TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess(t *testing.T) {
	h := testkit.TempDir(t)

	repoID := "kr://acme/public/warehouse"
	body(t, kc(h, "init"))
	seedRepo(t, h, repoID)
	body(t, kc(h, "put", testObjectReference(t, repoID, "Table:payments", "/structure"), "--command-id", "structure", "--value", `{"columns":["id"]}`))
	body(t, kc(h, "put", testObjectReference(t, repoID, "Table:payments", "/permissions/user:bob"), "--command-id", "source-grant", "--value", `{"privileges":["SELECT"]}`, "--kind", "Member"))
	body(t, kc(h, "dataset", "define", "--dataset", "warehouse", "--revision", "1",
		"--source", repoID+"=refs/heads/main"))
	// A source-system permissions Aspect still grants no kc read access.
	expectCode(t, kc(h, "read", testObjectReference(t, repoID, "Table:payments", ""), "--as", "bob", "--dataset", "warehouse"), "FORBIDDEN")
	expectCode(t, kc(h, "allowed", "--principal", "bob", "--cmd", "read", "--repo", repoID), "FORBIDDEN")

	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read-workspace", "--dataset", "warehouse"))
	values := asMap(t, body(t, kc(h, "read", testObjectReference(t, repoID, "Table:payments", ""), "--as", "bob", "--dataset", "warehouse")))
	if values["reference"] != testObjectReference(t, repoID, "Table:payments", "") {
		t.Fatalf("dataset file.read must deliver listed files: %#v", values)
	}
	expectCode(t, kc(h, "read", testObjectReference(t, repoID, "Table:payments", ""), "--as", "bob"), "FORBIDDEN")
	expectCode(t, kc(h, "allowed", "--principal", "bob", "--cmd", "read", "--repo", repoID), "FORBIDDEN")

	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read", "--repo", repoID))
	values = asMap(t, body(t, kc(h, "read", testObjectReference(t, repoID, "Table:payments", ""), "--as", "bob", "--dataset", "warehouse")))
	if values["reference"] != testObjectReference(t, repoID, "Table:payments", "") {
		t.Fatalf("repository grant must not drop dataset consume: %#v", values)
	}
	memberResolved := body(t, kc(h, "resolve", "--as", "bob", "--dataset", "warehouse",
		"--object", "Table:payments", "--aspect", "permissions", "--member", "user:bob")).([]any)
	if len(memberResolved) != 1 || asMap(t, memberResolved[0])["status"] != "RESOLVED" ||
		asMap(t, memberResolved[0])["memberKey"] != "user:bob" {
		t.Fatalf("resolve --aspect --member: %#v", memberResolved)
	}
	expectCode(t, kc(h, "resolve", "--as", "bob", "--dataset", "warehouse",
		"--object", "Table:payments", "--member", "user:bob"), "USAGE_INVALID")
}

// TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository proves the
// federation rule users rely on for reproducibility: following an upstream
// selector changes the next Workspace pin, not the tree or HEAD of a different
// Repository that happens to cite the upstream objectmodel.
func TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository(t *testing.T) {
	h := testkit.TempDir(t)
	upstream := "kr://acme/public/handbook"
	personal := "kr://acme/personals/alice"
	body(t, kc(h, "init"))
	seedRepo(t, h, upstream)
	seedRepo(t, h, personal)
	upV1 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, upstream, "policy/oncall", ""), "--command-id", "up-v1", "--value", `{"version":1}`)))["result"])
	asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, personal, "notes/oncall", ""), "--command-id", "note-v1", "--value", `{"text":"my checklist"}`, "--origin-kind", "ASSERTION", "--source-ref", "kc://acme/public/handbook@"+upV1["newCommit"].(string)+"/policy/oncall")))["result"])
	body(t, kc(h, "dataset", "define", "--dataset", "desk", "--revision", "1",
		"--source", personal+"=refs/heads/main@",
		"--source", upstream+"=refs/heads/main@refs/handbook"))
	// Following an upstream selector only changes the next published Dataset
	// version; it does not rewrite another Repository that cites the upstream.
	beforePersonal := asMap(t, body(t, kc(h, "read", testObjectReference(t, personal, "notes/oncall", ""), "--dataset", "desk")))
	if beforePersonal["reference"] != testObjectReference(t, personal, "notes/oncall", "") {
		t.Fatal(beforePersonal)
	}
	personalCommit := asMap(t, beforePersonal)["commit"]

	upV2 := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, upstream, "policy/oncall", ""), "--command-id", "up-v2", "--value", `{"version":2}`)))["result"])
	frozen := asMap(t, body(t, kc(h, "read", testObjectReference(t, upstream, "policy/oncall", ""), "--dataset", "desk")))
	if frozen["reference"] != testObjectReference(t, upstream, "policy/oncall", "") || asMap(t, asMap(t, frozen)["value"])["version"] != float64(1) {
		t.Fatalf("published dataset must stay frozen until republish: %#v", frozen)
	}
	body(t, kc(h, "dataset", "define", "--dataset", "desk", "--revision", "2",
		"--source", personal+"=refs/heads/main@",
		"--source", upstream+"=refs/heads/main@refs/handbook"))
	upValues := asMap(t, body(t, kc(h, "read", testObjectReference(t, upstream, "policy/oncall", ""), "--dataset", "desk")))
	if upValues["reference"] != testObjectReference(t, upstream, "policy/oncall", "") || asMap(t, upValues)["commit"] != upV2["newCommit"] {
		t.Fatalf("republished dataset did not pick up upstream V2: %#v", upValues)
	}
	personalValues := asMap(t, body(t, kc(h, "read", testObjectReference(t, personal, "notes/oncall", ""), "--dataset", "desk")))
	if personalValues["reference"] != testObjectReference(t, personal, "notes/oncall", "") || asMap(t, personalValues)["commit"] != personalCommit {
		t.Fatalf("upstream update rewrote the referencing repository: %#v", personalValues)
	}
}

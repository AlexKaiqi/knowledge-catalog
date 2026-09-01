package cli_test

import (
	"path/filepath"
	"testing"

	"kc/internal/testkit"
)

// TestUserJourneyAttachExistingRepository proves authority attachment as a
// user operation. The default native Dolt authority is opened in place and
// registered in the Catalog; opening may bootstrap the Knowledge tables.
func TestUserJourneyAttachExistingRepository(t *testing.T) {
	h := testkit.TempDir(t)
	sourceHome := testkit.TempDir(t)
	source := filepath.Join(t.TempDir(), "team-notes")
	repoID := "kr://acme/teams/platform"
	body(t, kc(sourceHome, "init", "--catalog", "kr://acme/source-catalog"))
	body(t, kc(sourceHome, "repo-add", "--repo", repoID, "--dir", source))
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", repoID, "--dir", source))
	state := asMap(t, body(t, kc(h, "read", "--catalog", "kr://acme/catalog")))
	repositories := businessRepositories(state)
	if len(repositories) != 1 || repositories[0] != repoID {
		t.Fatalf("attached repository is not registered: %#v", state)
	}
}

// TestUserJourneyManageAgentAccess covers the whole access-management job as
// an owner experiences it: grant a Workspace and its member, inspect the
// decision, consume as the agent, revoke the Workspace grant, and observe the
// denial immediately. --workspace is intentional: it is the primary product
// vocabulary; Workspace is the only composition and permission scope.
func TestUserJourneyManageAgentAccess(t *testing.T) {
	h := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repoID := "kr://acme/public/runbooks"
	workspaceID := "oncall-agent"
	body(t, kc(h, "init", "--catalog", catalogID))
	body(t, kc(h, "repo-add", "--repo", repoID))
	body(t, kc(h, "put", "--command-id", "seed", "--repo", repoID,
		"--object", "runbook/payments", "--value", `{"text":"freeze traffic"}`))
	body(t, kc(h, "define-workspace", "--workspace", workspaceID, "--revision", "1",
		"--source", repoID+"=refs/heads/main"))

	workspaceRule := asMap(t, body(t, kc(h, "allow", "--principal", "agent",
		"--cmd", "read-workspace", "--catalog", catalogID, "--workspace", workspaceID)))
	if workspaceRule["workspace"] != workspaceID {
		t.Fatalf("--workspace scope was not stored on the rule: %#v", workspaceRule)
	}
	repoRule := asMap(t, body(t, kc(h, "allow", "--principal", "agent",
		"--cmd", "read", "--repo", repoID)))

	who := asMap(t, body(t, kc(h, "whoami", "--as", "agent")))
	if who["principal"] != "agent" {
		t.Fatal(who)
	}
	decision := asMap(t, body(t, kc(h, "allowed", "--principal", "agent",
		"--cmd", "read-workspace", "--catalog", catalogID, "--workspace", workspaceID)))
	if decision["allow"] != true || decision["ruleId"] != workspaceRule["id"] {
		t.Fatalf("unexpected allow decision: %#v rule %#v", decision, workspaceRule)
	}
	values := body(t, kc(h, "read", "--as", "agent", "--workspace", workspaceID,
		"--object", "runbook/payments")).([]any)
	if len(values) != 1 || asMap(t, asMap(t, values[0])["value"])["text"] != "freeze traffic" {
		t.Fatal(values)
	}

	body(t, kc(h, "revoke", "--id", workspaceRule["id"].(string)))
	expectCode(t, kc(h, "allowed", "--principal", "agent", "--cmd", "read-workspace",
		"--catalog", catalogID, "--workspace", workspaceID), "FORBIDDEN")
	expectCode(t, kc(h, "read", "--as", "agent", "--workspace", workspaceID,
		"--object", "runbook/payments"), "FORBIDDEN")

	// The repository grant is independent and remains until it is explicitly
	// revoked; revoking the Workspace grant must not silently delete it.
	rules := asMap(t, body(t, kc(h, "allowed")))
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
	catalogID := "kr://acme/catalog"
	repoID := "kr://acme/public/warehouse"
	body(t, kc(h, "init", "--catalog", catalogID))
	body(t, kc(h, "repo-add", "--repo", repoID))
	body(t, kc(h, "put", "--command-id", "structure", "--repo", repoID,
		"--object", "Table:payments", "--aspect", "structure", "--value", `{"columns":["id"]}`))
	body(t, kc(h, "put", "--command-id", "source-grant", "--repo", repoID,
		"--object", "Table:payments", "--aspect", "permissions", "--member", "user:bob",
		"--value", `{"privileges":["SELECT"]}`))
	body(t, kc(h, "define-workspace", "--workspace", "warehouse", "--revision", "1",
		"--source", repoID+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read-workspace",
		"--catalog", catalogID, "--workspace", "warehouse"))

	// A source-system permissions Aspect still grants no kc read access. The
	// Workspace read fails closed so denial cannot be mistaken for absence.
	expectCode(t, kc(h, "read", "--as", "bob", "--workspace", "warehouse",
		"--object", "Table:payments"), "FORBIDDEN")
	expectCode(t, kc(h, "allowed", "--principal", "bob", "--cmd", "read", "--repo", repoID), "FORBIDDEN")

	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read", "--repo", repoID))
	values := body(t, kc(h, "read", "--as", "bob", "--workspace", "warehouse",
		"--object", "Table:payments")).([]any)
	if len(values) != 1 {
		t.Fatalf("explicit repository grant did not expose the knowledge: %#v", values)
	}
}

// TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository proves the
// federation rule users rely on for reproducibility: following an upstream
// selector changes the next Workspace pin, not the tree or HEAD of a different
// Repository that happens to cite the upstream knowledge.
func TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository(t *testing.T) {
	h := testkit.TempDir(t)
	upstream := "kr://acme/public/handbook"
	personal := "kr://acme/personals/alice"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", upstream))
	body(t, kc(h, "repo-add", "--repo", personal))
	upV1 := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "up-v1", "--repo", upstream,
		"--object", "policy/oncall", "--value", `{"version":1}`)))["result"])
	asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "note-v1", "--repo", personal,
		"--object", "notes/oncall", "--value", `{"text":"my checklist"}`,
		"--origin-kind", "ASSERTION", "--source-ref", "kc://acme/public/handbook@"+upV1["newCommit"].(string)+"/policy/oncall")))["result"])
	body(t, kc(h, "define-workspace", "--workspace", "desk", "--revision", "1",
		"--source", personal+"=refs/heads/main@",
		"--source", upstream+"=refs/heads/main@refs/handbook"))
	beforePersonal := body(t, kc(h, "read", "--workspace", "desk", "--object", "notes/oncall")).([]any)
	if len(beforePersonal) != 1 {
		t.Fatal(beforePersonal)
	}
	personalCommit := asMap(t, beforePersonal[0])["commit"]

	upV2 := asMap(t, asMap(t, body(t, kc(h, "put", "--command-id", "up-v2", "--repo", upstream,
		"--object", "policy/oncall", "--value", `{"version":2}`)))["result"])
	upValues := body(t, kc(h, "read", "--workspace", "desk", "--object", "policy/oncall")).([]any)
	if len(upValues) != 1 || asMap(t, upValues[0])["commit"] != upV2["newCommit"] {
		t.Fatalf("fresh Workspace did not follow upstream V2: %#v", upValues)
	}
	personalValues := body(t, kc(h, "read", "--workspace", "desk", "--object", "notes/oncall")).([]any)
	if len(personalValues) != 1 || asMap(t, personalValues[0])["commit"] != personalCommit {
		t.Fatalf("upstream update rewrote the referencing repository: %#v", personalValues)
	}
}

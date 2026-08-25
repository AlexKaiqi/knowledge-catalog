package cli_test

import (
	"encoding/base64"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/cli"
	"kc/internal/gitdir"
	"kc/internal/testkit"
)

// TestUserJourneyLinkExistingRepository proves --link as a user operation,
// rather than only parsing the flag: kc clones an existing Git repository,
// admits it to the Catalog, exposes its files through a Workspace, and leaves
// the source repository untouched.
func TestUserJourneyLinkExistingRepository(t *testing.T) {
	h := testkit.TempDir(t)
	source := filepath.Join(t.TempDir(), "team-notes")
	git, err := gitdir.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "welcome.md"), []byte("shared knowledge\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := git.StageAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Commit(gitdir.Signature{Message: "seed knowledge"}, false); err != nil {
		t.Fatal(err)
	}
	sourceHead, ok := git.Rev("HEAD")
	if !ok {
		t.Fatal("source repository has no HEAD")
	}

	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	repoID := "kr://acme/teams/platform"
	body(t, kc(h, "mount", repoID, "--link", source))
	body(t, kc(h, "define-workspace", "--workspace", "platform-agent", "--revision", "1",
		"--source", repoID+"=refs/heads/main@"))
	read := asMap(t, body(t, kc(h, "vfs-read", "--workspace", "platform-agent", "--path", "welcome.md")))
	content, err := base64.StdEncoding.DecodeString(read["content"].(string))
	if err != nil || string(content) != "shared knowledge\n" {
		t.Fatalf("linked repository was not consumable: %q %v", content, err)
	}
	if after, ok := git.Rev("HEAD"); !ok || after != sourceHead {
		t.Fatalf("mount --link changed source HEAD: before=%s after=%s", sourceHead, after)
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

	values := body(t, kc(h, "read", "--as", "bob", "--workspace", "warehouse",
		"--object", "Table:payments")).([]any)
	if len(values) != 0 {
		t.Fatalf("source-system GRANT leaked into kc authorization: %#v", values)
	}
	expectCode(t, kc(h, "allowed", "--principal", "bob", "--cmd", "read", "--repo", repoID), "FORBIDDEN")

	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read", "--repo", repoID))
	values = body(t, kc(h, "read", "--as", "bob", "--workspace", "warehouse",
		"--object", "Table:payments")).([]any)
	if len(values) != 1 {
		t.Fatalf("explicit repository grant did not expose the knowledge: %#v", values)
	}
}

// TestUserJourneyCrossRepoWriteReportsPartialOutcome exercises the contract a
// user needs when editing two mounts: each repository is its own transaction.
// The first commit remains applied when the second races, and the failed
// mount stays dirty instead of being silently discarded.
func TestUserJourneyCrossRepoWriteReportsPartialOutcome(t *testing.T) {
	h := testkit.TempDir(t)
	personal := "kr://acme/a-personal"
	shared := "kr://acme/z-shared"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", personal))
	body(t, kc(h, "repo-add", "--repo", shared))
	body(t, kc(h, "define-workspace", "--workspace", "desk", "--revision", "1",
		"--source", personal+"=refs/heads/main@",
		"--source", shared+"=refs/heads/main@refs/shared"))

	dest := filepath.Join(t.TempDir(), "desk")
	body(t, kc(h, "checkout", "--workspace", "desk", "--to", dest))
	if err := os.WriteFile(filepath.Join(dest, "personal.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sharedDraft := filepath.Join(dest, "refs", "shared", "shared.md")
	if err := os.WriteFile(sharedDraft, []byte("review me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Move only the shared repository after checkout. Sorting by repository id
	// makes personal commit first and shared race second.
	body(t, kc(h, "put", "--command-id", "move-shared", "--repo", shared,
		"--object", "note/upstream", "--value", `{"v":2}`))
	result := asMap(t, body(t, kc(h, "commit", "--workspace", "desk", "--to", dest,
		"--command-id", "two-mounts", "--message", "edit both")))
	rows := result["commits"].([]any)
	if len(rows) != 2 {
		t.Fatalf("one outcome per dirty repository is required: %#v", result)
	}
	applied, failed := 0, 0
	for _, raw := range rows {
		row := asMap(t, raw)
		if row["error"] != nil && row["error"] != "" {
			failed++
			continue
		}
		receipt := asMap(t, row["receipt"])
		if receipt["disposition"] == "APPLIED" {
			applied++
		}
	}
	if applied != 1 || failed != 1 {
		t.Fatalf("want one applied and one explicit failure: %#v", result)
	}

	readPersonal := asMap(t, body(t, kc(h, "vfs-read", "--workspace", "desk", "--path", "personal.md")))
	content, err := base64.StdEncoding.DecodeString(readPersonal["content"].(string))
	if err != nil || string(content) != "mine\n" {
		t.Fatalf("first repository was rolled back or corrupted: %q %v", content, err)
	}
	status := asMap(t, body(t, kc(h, "status", "--workspace", "desk", "--to", dest)))
	var sharedDirty bool
	for _, raw := range status["mounts"].([]any) {
		row := asMap(t, raw)
		if row["repository"] == shared && row["dirty"] == true {
			sharedDirty = true
		}
	}
	if !sharedDirty {
		t.Fatalf("failed mount edit must remain recoverable: %#v", status)
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

func TestUserJourneyFrozenCommandsDoNotPretendToWork(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	for _, verb := range []string{"capabilities", "expand-relations", "watch-updates", "list-tree", "reconcile", "connector-run"} {
		expectCode(t, kc(h, verb), "USAGE_INVALID")
	}
}

// TestUserJourneyGovernedPublishOverHTTP proves that the HTTP facade can run
// the same shared-knowledge lifecycle as the CLI, including inspection and a
// published update becoming visible to the next consumer.
func TestUserJourneyGovernedPublishOverHTTP(t *testing.T) {
	h := testkit.TempDir(t)
	server := httptest.NewServer(cli.HTTPHandler(h))
	t.Cleanup(server.Close)
	repoID := "kr://acme/public/policies"

	mustPostVerb(t, server.URL, "init", map[string]any{"catalog": "kr://acme/catalog"})
	mustPostVerb(t, server.URL, "repo-add", map[string]any{"repo": repoID})
	mustPostVerb(t, server.URL, "put", map[string]any{
		"command-id": "seed", "repo": repoID, "object": "policy/refunds", "value": map[string]any{"version": 1},
	})
	mustPostVerb(t, server.URL, "define-workspace", map[string]any{
		"workspace": "policy-agent", "revision": 1, "source": []string{repoID + "=refs/heads/main"},
	})
	checkoutDir := filepath.Join(t.TempDir(), "http-checkout")
	checkout := mustPostVerb(t, server.URL, "checkout", map[string]any{"workspace": "policy-agent", "to": checkoutDir})
	if checkout["workspaceId"] != "policy-agent" {
		t.Fatalf("HTTP checkout did not match CLI semantics: %#v", checkout)
	}
	resolved := mustPostVerb(t, server.URL, "resolve", map[string]any{"workspace": "policy-agent"})
	if resolved["pinId"] == "" {
		t.Fatal(resolved)
	}
	inspected := mustPostVerb(t, server.URL, "inspect", map[string]any{"workspace": "policy-agent"})
	if asMap(t, inspected["pin"])["pinId"] == "" {
		t.Fatal(inspected)
	}

	proposal := mustPostVerb(t, server.URL, "propose", map[string]any{
		"proposal-id": "PR-http", "repo": repoID,
		"target": "refs/heads/main", "candidate": "refs/heads/candidates/PR-http",
		"object": "policy/refunds", "value": map[string]any{"version": 2},
	})
	preview := mustPostVerb(t, server.URL, "preview", map[string]any{
		"proposal": proposal["proposalId"], "workspace": "policy-agent",
	})
	validation := mustPostVerb(t, server.URL, "record-validation", map[string]any{
		"preview": preview["previewId"], "suite": "approval:owner", "outcome": "PASSED",
	})
	mustPostVerb(t, server.URL, "merge", map[string]any{
		"proposal": proposal["proposalId"], "preview": preview["previewId"], "validation": validation["reportId"],
	})
	code, payload, raw := httpAny(t, server, "read", map[string]any{
		"workspace": "policy-agent", "object": "policy/refunds",
	}, "")
	if code != 200 {
		t.Fatalf("read status %d: %s", code, raw)
	}
	values := payload.([]any)
	if len(values) != 1 || asMap(t, asMap(t, values[0])["value"])["version"] != float64(2) {
		t.Fatalf("next HTTP consumer did not see the merge: %#v", values)
	}
}

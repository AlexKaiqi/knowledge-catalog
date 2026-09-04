package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/testkit"
)

func writeHookScript(t *testing.T, home, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPrePutDeniedLeavesNoCommit(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, "kr://acme/public/core")
	writeHookScript(t, h, "deny.sh", "#!/bin/sh\nexit 1\n")
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "deny.sh"))
	expectCode(t, kc(h, "put",
		"--command-id", "x",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
	), "HOOK_DENIED")
	expectCode(t, kc(h, "read", "--repo", "kr://acme/public/core", "--object", "policy/A", "--ref", "refs/heads/main"), "KNOWLEDGE_REF_UNRESOLVED")
}

func TestReplayedSkipsHook(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, "kr://acme/public/core")
	writeHookScript(t, h, "count.sh", "#!/bin/sh\necho x >> count.txt\n")
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "count.sh"))
	put := asMap(t, body(t, kc(h, "put",
		"--command-id", "sync-1",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
	)))
	if put["disposition"] != "APPLIED" {
		t.Fatal(put)
	}
	replay := asMap(t, body(t, kc(h, "put",
		"--command-id", "sync-1",
		"--repo", "kr://acme/public/core",
		"--object", "policy/A",
		"--value", `{"v":1}`,
	)))
	if replay["disposition"] != "REPLAYED" {
		t.Fatal(replay)
	}
	raw, err := os.ReadFile(filepath.Join(h, "count.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "x"); got != 1 {
		t.Fatalf("hook ran %d times, want 1", got)
	}
}

func TestMergeGateMissingSuiteAndPreviewMove(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-103", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	body(t, kc(h, "gate-add", "--on", "merge", "--repo", core, "--require", "suite:metrics-contract"))

	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-103", "--value", `{"v":2}`,
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--workspace", "agent")))
	body(t, kc(h, "validate", "--preview", preview["previewId"].(string)))
	body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "S7", "--outcome", "PASSED"))
	expectCode(t, kc(h, "merge", "--proposal", "PR-1", "--preview", preview["previewId"].(string)), "GATE_UNSATISFIED")

	body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "metrics-contract", "--outcome", "PASSED"))
	moved := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1b", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-103", "--value", `{"v":3}`,
	)))
	expectCode(t, kc(h, "merge", "--proposal", "PR-1", "--preview", preview["previewId"].(string)), "CANDIDATE_MOVED")

	preview2 := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1b", "--workspace", "agent")))
	expectCode(t, kc(h, "merge", "--proposal", "PR-1b", "--preview", preview2["previewId"].(string)), "GATE_UNSATISFIED")
	body(t, kc(h, "record-validation", "--preview", preview2["previewId"].(string), "--suite", "metrics-contract", "--outcome", "PASSED"))
	merged := asMap(t, body(t, kc(h, "merge", "--proposal", "PR-1b", "--preview", preview2["previewId"].(string))))
	if merged["commitId"] != moved["candidateCommit"] {
		t.Fatal(merged, moved, proposal)
	}
}

func TestMergeAuthorizationDerivesRepositoryAndRefFromProposal(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	body(t, kc(h,
		"propose", "--proposal-id", "PR-auth", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-auth",
		"--object", "policy/P-1", "--value", `{"v":2}`,
	))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-auth", "--workspace", "agent")))
	validation := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	body(t, kc(h, "allow", "--principal", "reviewer", "--cmd", "merge", "--repo", core, "--ref", "refs/heads/main"))

	merged := asMap(t, body(t, kc(h,
		"merge", "--as", "reviewer", "--proposal", "PR-auth",
		"--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string),
	)))
	if merged["repository"] != core || merged["targetRef"] != "refs/heads/main" {
		t.Fatal("merge receipt must explain its derived authorization scope", merged)
	}
}

func TestValidationAuthorizationDerivesWorkspaceFromPreview(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	catalogID := "kr://acme/catalog"
	kc(h, "init", "--catalog", catalogID)
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	body(t, kc(h,
		"propose", "--proposal-id", "PR-validation-auth", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-validation-auth",
		"--object", "policy/P-1", "--value", `{"v":2}`,
	))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-validation-auth", "--workspace", "agent")))
	previewID := preview["previewId"].(string)
	body(t, kc(h, "allow", "--principal", "reviewer", "--cmd", "validate,record-validation", "--catalog", catalogID, "--workspace", "agent"))

	report := asMap(t, body(t, kc(h, "validate", "--as", "reviewer", "--preview", previewID)))
	if report["outcome"] != "PASSED" {
		t.Fatal(report)
	}
	body(t, kc(h, "record-validation", "--as", "reviewer", "--preview", previewID, "--suite", "approval", "--outcome", "PASSED"))

	// A caller-supplied Workspace cannot move authorization away from the
	// immutable Preview scope.
	body(t, kc(h, "allow", "--principal", "other", "--cmd", "validate", "--catalog", catalogID, "--workspace", "other"))
	expectCode(t, kc(h, "validate", "--as", "other", "--preview", previewID, "--workspace", "other"), "FORBIDDEN")
}

func TestPostDefineWorkspacePointersOnlyAndFailureDoesNotRollback(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"secret":true}`))

	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	body(t, kc(h, "hook-add", "--on", "define-workspace", "--phase", "post", "--url", srv.URL))
	defined := asMap(t, body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main")))
	var event map[string]any
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatal(err, string(got))
	}
	if event["action"] != "workspace.manage" || defined["workspaceId"] != "agent" {
		t.Fatal(event, defined)
	}
	if _, ok := event["value"]; ok {
		t.Fatal("post event must not carry object bodies", event)
	}
	if _, ok := event["secret"]; ok {
		t.Fatal(event)
	}

	body(t, kc(h, "hook-add", "--on", "define-workspace", "--phase", "post", "--url", "http://127.0.0.1:1/nope"))
	again := asMap(t, body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "2", "--source", core+"=refs/heads/main")))
	if again["workspaceId"] != "agent" {
		t.Fatal(again)
	}
	if _, err := os.Stat(filepath.Join(h, "hook-outbox.jsonl")); err != nil {
		t.Fatal("expected outbox after post failure", err)
	}
}

func TestHookAndGateConfigCRUD(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init", "--catalog", "kr://acme/catalog")
	writeHookScript(t, h, "ok.sh", "#!/bin/sh\nexit 0\n")
	hooked := asMap(t, body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "ok.sh")))
	if hooked["id"] == "" || hooked["on"] != "writer.commit" {
		t.Fatal(hooked)
	}
	expectCode(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--url", "http://example"), "USAGE_INVALID")
	expectCode(t, kc(h, "hook-add", "--on", "read", "--phase", "post", "--run", "ok.sh"), "USAGE_INVALID")
	listed := asMap(t, body(t, kc(h, "hook-ls", "--on", "writer.commit")))
	if len(listed["bindings"].([]any)) != 1 {
		t.Fatal(listed)
	}
	body(t, kc(h, "hook-rm", "--id", hooked["id"].(string)))
	empty := asMap(t, body(t, kc(h, "hook-ls")))
	if len(empty["bindings"].([]any)) != 0 {
		t.Fatal(empty)
	}
	expectMsg(t, kc(h, "hook-rm", "--id", "hk_missing"), "unknown hook")

	expectCode(t, kc(h, "gate-add", "--on", "merge", "--require", "validate"), "USAGE_INVALID")
	expectCode(t, kc(h, "gate-add", "--on", "promote", "--require", "suite:contract"), "USAGE_INVALID")
	expectCode(t, kc(h, "gate-add", "--on", "put", "--repo", "kr://acme/public/core", "--require", "validate"), "USAGE_INVALID")
	gated := asMap(t, body(t, kc(h, "gate-add", "--on", "merge", "--repo", "kr://acme/public/core", "--require", "validate,suite:lint")))
	if gated["id"] == "" {
		t.Fatal(gated)
	}
	rules := asMap(t, body(t, kc(h, "gate-ls", "--on", "merge", "--repo", "kr://acme/public/core")))
	if len(rules["rules"].([]any)) != 1 {
		t.Fatal(rules)
	}
	body(t, kc(h, "gate-rm", "--id", gated["id"].(string)))
	cleared := asMap(t, body(t, kc(h, "gate-ls")))
	if len(cleared["rules"].([]any)) != 0 {
		t.Fatal(cleared)
	}
}

func TestPreMergeDoesNotSatisfyGate(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	writeHookScript(t, h, "ok.sh", "#!/bin/sh\nexit 0\n")
	body(t, kc(h, "hook-add", "--on", "merge", "--phase", "pre", "--repo", core, "--run", "ok.sh"))
	body(t, kc(h, "gate-add", "--on", "merge", "--repo", core, "--require", "suite:metrics-contract"))
	body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-1", "--value", `{"v":2}`,
	))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--workspace", "agent")))
	body(t, kc(h, "validate", "--preview", preview["previewId"].(string)))
	expectCode(t, kc(h, "merge", "--proposal", "PR-1", "--preview", preview["previewId"].(string)), "GATE_UNSATISFIED")
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "put,remove,commit", "--repo", core))
	expectCode(t, kc(h, "merge", "--as", "bot", "--proposal", "PR-1", "--preview", preview["previewId"].(string)), "FORBIDDEN")
}

func TestReadPathAndPutIgnoreGates(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	writeHookScript(t, h, "count.sh", "#!/bin/sh\necho x >> count.txt\n")
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", core, "--run", "count.sh"))
	body(t, kc(h, "gate-add", "--on", "merge", "--repo", core, "--require", "suite:lint"))
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "read", "--repo", core, "--object", "policy/P-1", "--ref", "refs/heads/main"))
	raw, err := os.ReadFile(filepath.Join(h, "count.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "x"); got != 1 {
		t.Fatalf("read must not fire put hook, got %d", got)
	}
}

func TestMergeStillNeedsValidationWithoutGates(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-1", "--value", `{"v":2}`,
	))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--workspace", "agent")))
	expectMsg(t, kc(h, "merge", "--proposal", "PR-1", "--preview", preview["previewId"].(string)), "merge needs stored")
}

func TestFailedSuiteAndOtherRepoGate(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	body(t, kc(h, "gate-add", "--on", "merge", "--repo", "kr://acme/semantic", "--require", "suite:metrics-contract"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-1", "--value", `{"v":2}`,
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--workspace", "agent")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	merged := asMap(t, body(t, kc(h, "merge", "--proposal", "PR-1", "--preview", preview["previewId"].(string), "--validation", structural["reportId"].(string))))
	if merged["commitId"] != proposal["candidateCommit"] {
		t.Fatal("other-repo gate must not block this merge", merged, proposal)
	}

	body(t, kc(h, "gate-add", "--on", "merge", "--repo", core, "--require", "suite:lint"))
	proposal2 := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-2", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-2",
		"--object", "policy/P-1", "--value", `{"v":3}`,
	)))
	preview2 := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-2", "--workspace", "agent")))
	body(t, kc(h, "record-validation", "--preview", preview2["previewId"].(string), "--suite", "lint", "--outcome", "FAILED"))
	expectCode(t, kc(h, "merge", "--proposal", "PR-2", "--preview", preview2["previewId"].(string)), "GATE_UNSATISFIED")
	body(t, kc(h, "record-validation", "--preview", preview2["previewId"].(string), "--suite", "lint", "--outcome", "PASSED"))
	ok := asMap(t, body(t, kc(h, "merge", "--proposal", "PR-2", "--preview", preview2["previewId"].(string))))
	if ok["commitId"] != proposal2["candidateCommit"] {
		t.Fatal(ok, proposal2)
	}
}

func TestValidatePreviewRecordsStructure(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "seed", "--repo", core, "--object", "policy/P-1", "--value", `{"v":1}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	body(t, kc(h,
		"propose", "--proposal-id", "PR-val", "--repo", core,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-val",
		"--object", "policy/P-1", "--value", `{"v":2}`,
	))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-val", "--workspace", "agent")))
	report := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if report["outcome"] != "PASSED" || report["suiteRevision"] != "structure" || report["previewId"] != preview["previewId"] {
		t.Fatal(report)
	}
}

func TestPostPutPointersOnly(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init", "--catalog", "kr://acme/catalog")
	seedRepo(t, h, core)
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "post", "--repo", core, "--url", srv.URL))
	put := asMap(t, body(t, kc(h, "put",
		"--command-id", "seed",
		"--repo", core,
		"--object", "policy/P-1",
		"--value", `{"secret":true}`,
	)))
	var event map[string]any
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatal(err, string(got))
	}
	if event["action"] != "writer.commit" || event["newCommit"] == nil || event["disposition"] != "APPLIED" {
		t.Fatal(event, put)
	}
	if _, ok := event["secret"]; ok {
		t.Fatal(event)
	}
	if _, ok := event["value"]; ok {
		t.Fatal(event)
	}
}

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

// putOnRef writes one object on ref and returns the new branch commit.
func putOnRef(t *testing.T, home, repo, ref, object, commandID, value string) string {
	t.Helper()
	put := asMap(t, body(t, kc(home, "put", testObjectReference(t, repo, object, ""), "--repo", repo,
		"--command-id", commandID, "--ref", ref, "--value", value)))
	commit, _ := asMap(t, put["result"])["newCommit"].(string)
	if put["disposition"] != "APPLIED" || commit == "" {
		t.Fatal(put)
	}
	return commit
}

func mainHead(t *testing.T, home, repo string) string {
	t.Helper()
	head := asMap(t, body(t, kc(home, "writer-head", "--repo", repo)))
	commit, _ := head["commit"].(string)
	if commit == "" {
		t.Fatal(head)
	}
	return commit
}

func TestPrePutDeniedLeavesNoCommit(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	writeHookScript(t, h, "deny.sh", "#!/bin/sh\nexit 1\n")
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "deny.sh"))
	expectCode(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--repo", "kr://acme/public/core", "--command-id", "x", "--value", `{"v":1}`), "HOOK_DENIED")
	expectCode(t, kc(h, "read", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--repo", "kr://acme/public/core", "--ref", "refs/heads/main"), "OBJECT_REF_UNRESOLVED")
}

func TestReplayedSkipsHook(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	writeHookScript(t, h, "count.sh", "#!/bin/sh\necho x >> count.txt\n")
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "count.sh"))
	put := asMap(t, body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--repo", "kr://acme/public/core", "--command-id", "sync-1", "--value", `{"v":1}`)))
	if put["disposition"] != "APPLIED" {
		t.Fatal(put)
	}
	replay := asMap(t, body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--repo", "kr://acme/public/core", "--command-id", "sync-1", "--value", `{"v":1}`)))
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

// TestMergePublishesOnlyTheReviewedBranchCommit replaces the retired
// review flow: a branch write stays off main, a reviewed commit
// that moved is refused, and the merge publishes exactly the branch head.
func TestMergePublishesOnlyTheReviewedBranchCommit(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-103", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	published := mainHead(t, h, core)

	branch := asMap(t, body(t, kc(h, "branch", "--repo", core, "--name", "review/PR-1")))
	if branch["ref"] != "refs/heads/review/PR-1" || branch["commitId"] != published {
		t.Fatal(branch)
	}
	expectCode(t, kc(h, "branch", "--repo", core, "--name", "review/PR-1"), "PRECONDITION_FAILED")
	reviewed := putOnRef(t, h, core, "refs/heads/review/PR-1", "policy/P-103", "pr-1-v2", `{"v":2}`)
	if mainHead(t, h, core) != published {
		t.Fatal("a branch write must not move refs/heads/main")
	}
	moved := putOnRef(t, h, core, "refs/heads/review/PR-1", "policy/P-103", "pr-1-v3", `{"v":3}`)

	expectCode(t, kc(h, "merge", "--command-id", "merge-stale", "--repo", core,
		"--from", "review/PR-1", "--commit", reviewed), "CANDIDATE_MOVED")
	if mainHead(t, h, core) != published {
		t.Fatal("a refused merge must leave main unchanged")
	}
	merged := asMap(t, body(t, kc(h, "merge", "--command-id", "merge-1", "--repo", core,
		"--from", "review/PR-1", "--commit", moved, "--expected", published)))
	result := asMap(t, merged["result"])
	if merged["disposition"] != "APPLIED" || result["commitId"] != moved || result["oldCommit"] != published {
		t.Fatal(merged)
	}
	replay := asMap(t, body(t, kc(h, "merge", "--command-id", "merge-1", "--repo", core,
		"--from", "review/PR-1", "--commit", moved, "--expected", published)))
	if replay["disposition"] != "REPLAYED" || asMap(t, replay["result"])["commitId"] != moved {
		t.Fatal(replay)
	}
	read := asMap(t, body(t, kc(h, "read", testObjectReference(t, core, "policy/P-103", ""), "--repo", core, "--ref", "refs/heads/main")))
	if !strings.Contains(string(mustJSON(t, read)), `"v":3`) {
		t.Fatal("main must serve the merged branch body", read)
	}
}

func TestMergeRefusesDivergedTarget(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	body(t, kc(h, "branch", "--repo", core, "--name", "review/diverged"))
	putOnRef(t, h, core, "refs/heads/review/diverged", "policy/P-1", "branch-v2", `{"v":2}`)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-2", ""), "--repo", core, "--command-id", "main-v1", "--value", `{"v":1}`))
	advanced := mainHead(t, h, core)

	expectCode(t, kc(h, "merge", "--command-id", "merge-diverged", "--repo", core, "--from", "review/diverged"), "NON_FAST_FORWARD")
	if mainHead(t, h, core) != advanced {
		t.Fatal("a non-fast-forward merge must leave main unchanged")
	}
}

func TestMergeAuthorizationDerivesTargetRefFromInto(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	body(t, kc(h, "branch", "--repo", core, "--name", "review/auth"))
	putOnRef(t, h, core, "refs/heads/review/auth", "policy/P-1", "auth-v2", `{"v":2}`)

	body(t, kc(h, "allow", "--principal", "elsewhere", "--cmd", "merge", "--repo", core, "--ref", "refs/heads/release"))
	expectCode(t, kc(h, "merge", "--as", "elsewhere", "--command-id", "merge-elsewhere", "--repo", core, "--from", "review/auth"), "FORBIDDEN")

	body(t, kc(h, "allow", "--principal", "reviewer", "--cmd", "merge", "--repo", core, "--ref", "refs/heads/main"))
	merged := asMap(t, body(t, kc(h, "merge", "--as", "reviewer", "--command-id", "merge-auth", "--repo", core, "--from", "review/auth")))
	result := asMap(t, merged["result"])
	if result["repositoryId"] != core || result["targetRef"] != "refs/heads/main" {
		t.Fatal("merge receipt must explain its derived authorization scope", merged)
	}
}

func TestBranchAuthorizationDerivesRefFromName(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	body(t, kc(h, "allow", "--principal", "drafter", "--cmd", "branch", "--repo", core, "--ref", "refs/heads/review/a"))

	created := asMap(t, body(t, kc(h, "branch", "--as", "drafter", "--repo", core, "--name", "review/a")))
	if created["ref"] != "refs/heads/review/a" {
		t.Fatal(created)
	}
	expectCode(t, kc(h, "branch", "--as", "drafter", "--repo", core, "--name", "review/b"), "FORBIDDEN")
	expectCode(t, kc(h, "branch", "--repo", core, "--name", "review/c", "--from", "main", "--commit", "abc"), "USAGE_INVALID")
}

func TestPostDefineDatasetPointersOnlyAndFailureDoesNotRollback(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"secret":true}`))

	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	body(t, kc(h, "hook-add", "--on", "dataset.manage", "--phase", "post", "--url", srv.URL))
	defined := asMap(t, body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main")))
	var event map[string]any
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatal(err, string(got))
	}
	if event["action"] != "dataset.manage" || defined["setId"] != "agent" {
		t.Fatal(event, defined)
	}
	if _, ok := event["value"]; ok {
		t.Fatal("post event must not carry object bodies", event)
	}
	if _, ok := event["secret"]; ok {
		t.Fatal(event)
	}

	body(t, kc(h, "hook-add", "--on", "dataset.manage", "--phase", "post", "--url", "http://127.0.0.1:1/nope"))
	again := asMap(t, body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "2", "--source", core+"=refs/heads/main")))
	if again["setId"] != "agent" {
		t.Fatal(again)
	}
	if _, err := os.Stat(filepath.Join(h, "hook-outbox.jsonl")); err != nil {
		t.Fatal("expected outbox after post failure", err)
	}
}

func TestHookConfigCRUD(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	writeHookScript(t, h, "ok.sh", "#!/bin/sh\nexit 0\n")
	hooked := asMap(t, body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "ok.sh")))
	if hooked["id"] == "" || hooked["on"] != "writer.commit" {
		t.Fatal(hooked)
	}
	merging := asMap(t, body(t, kc(h, "hook-add", "--on", "merge", "--phase", "pre", "--repo", "kr://acme/public/core", "--run", "ok.sh")))
	if merging["id"] == "" || merging["on"] != "writer.merge" {
		t.Fatal(merging)
	}
	expectCode(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--url", "http://example"), "USAGE_INVALID")
	expectCode(t, kc(h, "hook-add", "--on", "read", "--phase", "post", "--run", "ok.sh"), "USAGE_INVALID")
	listed := asMap(t, body(t, kc(h, "hook-ls", "--on", "writer.commit")))
	if len(listed["bindings"].([]any)) != 1 {
		t.Fatal(listed)
	}
	body(t, kc(h, "hook-rm", "--id", hooked["id"].(string)))
	body(t, kc(h, "hook-rm", "--id", merging["id"].(string)))
	empty := asMap(t, body(t, kc(h, "hook-ls")))
	if len(empty["bindings"].([]any)) != 0 {
		t.Fatal(empty)
	}
	expectMsg(t, kc(h, "hook-rm", "--id", "hk_missing"), "unknown hook")
}

// TestPreMergeHookVetoesMerge is where business approval plugs in: the
// pre-merge hook can refuse, and write grants alone never authorize merge.
func TestPreMergeHookVetoesMerge(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	published := mainHead(t, h, core)
	body(t, kc(h, "branch", "--repo", core, "--name", "review/veto"))
	putOnRef(t, h, core, "refs/heads/review/veto", "policy/P-1", "veto-v2", `{"v":2}`)

	writeHookScript(t, h, "deny.sh", "#!/bin/sh\nexit 1\n")
	denied := asMap(t, body(t, kc(h, "hook-add", "--on", "merge", "--phase", "pre", "--repo", core, "--run", "deny.sh")))
	expectCode(t, kc(h, "merge", "--command-id", "merge-veto", "--repo", core, "--from", "review/veto"), "HOOK_DENIED")
	if mainHead(t, h, core) != published {
		t.Fatal("a vetoed merge must leave main unchanged")
	}
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "put,remove,commit,branch", "--repo", core))
	expectCode(t, kc(h, "merge", "--as", "bot", "--command-id", "merge-bot", "--repo", core, "--from", "review/veto"), "FORBIDDEN")

	body(t, kc(h, "hook-rm", "--id", denied["id"].(string)))
	merged := asMap(t, body(t, kc(h, "merge", "--command-id", "merge-approved", "--repo", core, "--from", "review/veto")))
	if merged["disposition"] != "APPLIED" || mainHead(t, h, core) == published {
		t.Fatal(merged)
	}
}

func TestReadPathAndPutIgnoreMergeHooks(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	writeHookScript(t, h, "count.sh", "#!/bin/sh\necho x >> count.txt\n")
	writeHookScript(t, h, "merge.sh", "#!/bin/sh\necho m >> merge.txt\n")
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "pre", "--repo", core, "--run", "count.sh"))
	body(t, kc(h, "hook-add", "--on", "merge", "--phase", "pre", "--repo", core, "--run", "merge.sh"))
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	body(t, kc(h, "read", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--ref", "refs/heads/main"))
	raw, err := os.ReadFile(filepath.Join(h, "count.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "x"); got != 1 {
		t.Fatalf("read must not fire put hook, got %d", got)
	}
	if _, err := os.Stat(filepath.Join(h, "merge.txt")); !os.IsNotExist(err) {
		t.Fatal("put and read must not fire merge hooks", err)
	}
}

func TestOtherRepositoryMergeHookDoesNotBlock(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"v":1}`))
	writeHookScript(t, h, "deny.sh", "#!/bin/sh\nexit 1\n")
	body(t, kc(h, "hook-add", "--on", "merge", "--phase", "pre", "--repo", "kr://acme/semantic", "--run", "deny.sh"))
	body(t, kc(h, "branch", "--repo", core, "--name", "review/other"))
	head := putOnRef(t, h, core, "refs/heads/review/other", "policy/P-1", "other-v2", `{"v":2}`)
	merged := asMap(t, body(t, kc(h, "merge", "--command-id", "merge-other", "--repo", core, "--from", "review/other")))
	if asMap(t, merged["result"])["commitId"] != head {
		t.Fatal("other-repo hook must not block this merge", merged)
	}
}

func TestPostPutPointersOnly(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	body(t, kc(h, "hook-add", "--on", "put", "--phase", "post", "--repo", core, "--url", srv.URL))
	put := asMap(t, body(t, kc(h, "put", testObjectReference(t, core, "policy/P-1", ""), "--repo", core, "--command-id", "seed", "--value", `{"secret":true}`)))
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

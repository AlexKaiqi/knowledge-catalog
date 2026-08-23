package cli_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"kc/internal/testkit"
)

// TestVFSFlow exercises vfs-list/vfs-read/vfs-write end to end through the
// real CLI dispatch path (the same one kc serve's POST /v1/<verb> uses): a
// root mount plus a nested mount, list across both, read a file that exists
// only via routing, then write one and read the write back. This is the
// primitive an external agent harness's own filesystem provider calls per
// file (docs/COMPOSITION.md) — no checkout ever touches disk here.
func TestVFSFlow(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	body(t, kc(h, "repo-add", "--repo", semantic))

	// Seed each member with a plain file via vfs-write itself, proving the
	// write surface works before anything else depends on it.
	seedAliceCommit := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "seed-alice", "--repo", alice, "--object", "note/seed", "--value", `{"v":1}`,
	)))["result"])["newCommit"].(string)
	seedSemanticCommit := asMap(t, asMap(t, body(t, kc(h, "put",
		"--command-id", "seed-semantic", "--repo", semantic, "--object", "metric/seed", "--value", `{"v":1}`,
	)))["result"])["newCommit"].(string)
	_ = seedAliceCommit
	_ = seedSemanticCommit

	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	))

	// vfs-write: create a new file at the root mount, and one in the nested mount.
	content := base64.StdEncoding.EncodeToString([]byte("draft content\n"))
	body(t, kc(h, "vfs-write", "--workspace", "notes", "--command-id", "vfs-1",
		"--path", "analysis/churn.md", "--content", content))
	metricContent := base64.StdEncoding.EncodeToString([]byte("daily actives\n"))
	body(t, kc(h, "vfs-write", "--workspace", "notes", "--command-id", "vfs-2",
		"--path", "refs/semantic/metrics/dau.md", "--content", metricContent))

	// vfs-read must route each virtual path to the repository that actually
	// owns it and return exact bytes.
	read1 := asMap(t, body(t, kc(h, "vfs-read", "--workspace", "notes", "--path", "analysis/churn.md")))
	if read1["repository"] != alice {
		t.Fatalf("root mount file must route to alice: %#v", read1)
	}
	got1, err := base64.StdEncoding.DecodeString(read1["content"].(string))
	if err != nil || string(got1) != "draft content\n" {
		t.Fatalf("%q %v", got1, err)
	}

	read2 := asMap(t, body(t, kc(h, "vfs-read", "--workspace", "notes", "--path", "refs/semantic/metrics/dau.md")))
	if read2["repository"] != semantic {
		t.Fatalf("nested mount file must route to semantic: %#v", read2)
	}
	got2, err := base64.StdEncoding.DecodeString(read2["content"].(string))
	if err != nil || string(got2) != "daily actives\n" {
		t.Fatalf("%q %v", got2, err)
	}

	// vfs-list must cover both mounts under their composed virtual paths.
	listing := asMap(t, body(t, kc(h, "vfs-list", "--workspace", "notes")))
	entries, ok := listing["entries"].([]any)
	if !ok {
		t.Fatalf("entries: %#v", listing)
	}
	paths := map[string]bool{}
	for _, raw := range entries {
		e := asMap(t, raw)
		paths[e["path"].(string)] = true
	}
	if !paths["analysis/churn.md"] {
		t.Fatalf("listing must include the root mount's new file: %v", paths)
	}
	if !paths["refs/semantic/metrics/dau.md"] {
		t.Fatalf("listing must include the nested mount's new file: %v", paths)
	}

	// A path nobody's mount owns must be refused, not silently guessed.
	failed := kc(h, "vfs-read", "--workspace", "notes", "--path", "nowhere/at/all.md")
	if failed.Status == 0 {
		t.Fatal("an unowned path must fail vfs-read")
	}
}

// Rewriting the same command-id with the same content and the same pinned
// --base replays; a different payload under the same command-id is an
// idempotency conflict — the same contract every other Writer surface gives.
// --base matters here: without it each call would resolve a fresh (moved)
// base and never look like the same request twice.
func TestVFSWriteIdempotency(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1", "--source", alice+"=refs/heads/main@"))

	resolved := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	base := asMap(t, resolved["repositories"])[alice].(string)

	content := base64.StdEncoding.EncodeToString([]byte("draft\n"))
	first := asMap(t, asMap(t, body(t, kc(h, "vfs-write", "--workspace", "notes", "--command-id", "cmd-1",
		"--path", "note.md", "--content", content, "--base", base)))["result"])
	second := asMap(t, asMap(t, body(t, kc(h, "vfs-write", "--workspace", "notes", "--command-id", "cmd-1",
		"--path", "note.md", "--content", content, "--base", base)))["result"])
	if first["newCommit"] != second["newCommit"] {
		t.Fatalf("replay must return the same commit: %v != %v", first["newCommit"], second["newCommit"])
	}

	other := base64.StdEncoding.EncodeToString([]byte("different\n"))
	conflict := kc(h, "vfs-write", "--workspace", "notes", "--command-id", "cmd-1", "--path", "note.md", "--content", other, "--base", base)
	if conflict.Status == 0 {
		t.Fatal("same command-id with a different payload must be an idempotency conflict")
	}
}

// A stale --base (the ref advanced since the caller last read) is
// NON_FAST_FORWARD, not IDEMPOTENCY_CONFLICT: a fresh command-id racing an
// outdated read is a real conflict to resolve, not a payload mismatch.
func TestVFSWriteRejectsStaleBase(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1", "--source", alice+"=refs/heads/main@"))

	resolved := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	base := asMap(t, resolved["repositories"])[alice].(string)

	content := base64.StdEncoding.EncodeToString([]byte("first\n"))
	body(t, kc(h, "vfs-write", "--workspace", "notes", "--command-id", "cmd-a",
		"--path", "a.md", "--content", content, "--base", base))

	stale := base64.StdEncoding.EncodeToString([]byte("second\n"))
	result := kc(h, "vfs-write", "--workspace", "notes", "--command-id", "cmd-b",
		"--path", "b.md", "--content", stale, "--base", base)
	if result.Status == 0 {
		t.Fatal("writing against a base the ref has moved past must fail")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatal(err)
	}
	errObj := asMap(t, payload["error"])
	if errObj["code"] != "NON_FAST_FORWARD" {
		t.Fatalf("expected NON_FAST_FORWARD, got %v", errObj)
	}
}

func TestVFSWriteAsWithoutCommitGrant(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@"))
	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "notes"))
	body(t, kc(h, "allow", "--principal", "bob", "--cmd", "read", "--repo", alice))
	content := base64.StdEncoding.EncodeToString([]byte("owned\n"))
	body(t, kc(h, "vfs-write", "--workspace", "notes", "--command-id", "owner-1",
		"--path", "analysis/notes.md", "--content", content))
	expectCode(t, kc(h, "vfs-write", "--as", "bob", "--workspace", "notes", "--command-id", "bob-1",
		"--path", "analysis/hacked.md", "--content", content), "FORBIDDEN")
	got := asMap(t, body(t, kc(h, "vfs-read", "--as", "bob", "--workspace", "notes", "--path", "analysis/notes.md")))
	raw, err := base64.StdEncoding.DecodeString(got["content"].(string))
	if err != nil || string(raw) != "owned\n" {
		t.Fatalf("bob must still read: %q %v", raw, err)
	}
	viaWorkspace := asMap(t, body(t, kc(h, "vfs-read", "--as", "bob", "--workspace", "notes", "--path", "analysis/notes.md")))
	if viaWorkspace["content"] != got["content"] {
		t.Fatalf("--workspace must match a --workspace grant: %#v", viaWorkspace)
	}
}

package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// TestLoomWorkspaceFlow is docs/COMPOSITION.md §1.4 through the CLI:
// mount (including --dir), define-workspace with paths, checkout --to, edit,
// status, commit --workspace (RawWrite by path), sync. Also --as skips a mount
// at checkout time, and --pin replays a frozen ResolveWorkspace.
func TestLoomWorkspaceFlow(t *testing.T) {
	t.Skip("writable Git worktree checkout retired with FileGit; kcfs is the host projection")
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	body(t, kc(h, "repo-add", "--repo", semantic))
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	))

	dest := filepath.Join(t.TempDir(), "work")
	checked := asMap(t, body(t, kc(h, "checkout", "--workspace", "notes", "--to", dest)))
	if checked["workspaceId"] != "notes" {
		t.Fatal(checked)
	}
	if _, err := os.Stat(filepath.Join(dest, ".kc-pin.json")); err != nil {
		t.Fatal(err)
	}
	recipe, err := os.ReadFile(filepath.Join(dest, ".kc-workspace.yaml"))
	if err != nil {
		t.Fatal("root mount must carry .kc-workspace.yaml after define-workspace")
	}
	if !strings.Contains(string(recipe), "name: notes") {
		t.Fatalf("recipe: %s", recipe)
	}

	draft := filepath.Join(dest, "analysis", "retention.md")
	if err := os.MkdirAll(filepath.Dir(draft), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(draft, []byte("retention notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := asMap(t, body(t, kc(h, "status", "--workspace", "notes", "--to", dest)))
	mounts, _ := st["mounts"].([]any)
	dirty := false
	for _, raw := range mounts {
		m := asMap(t, raw)
		if m["repository"] == alice && m["dirty"] == true {
			dirty = true
		}
	}
	if !dirty {
		t.Fatalf("alice mount must be dirty: %#v", st)
	}

	committed := asMap(t, body(t, kc(h, "commit", "--workspace", "notes", "--to", dest,
		"--command-id", "ws-1", "--message", "add retention analysis")))
	rows, _ := committed["commits"].([]any)
	if len(rows) != 1 {
		t.Fatalf("one dirty mount → one receipt: %#v", committed)
	}
	receipt := asMap(t, asMap(t, rows[0])["receipt"])
	if receipt["disposition"] != "APPLIED" {
		t.Fatal(receipt)
	}

	got, err := os.ReadFile(draft)
	if err != nil || string(got) != "retention notes\n" {
		t.Fatalf("worktree must keep the committed file after reset: %q %v", got, err)
	}
	st2 := asMap(t, body(t, kc(h, "status", "--workspace", "notes", "--to", dest)))
	for _, raw := range st2["mounts"].([]any) {
		m := asMap(t, raw)
		if m["repository"] == alice && m["dirty"] == true {
			t.Fatalf("alice must be clean after commit: %#v", st2)
		}
	}

	expectMsg(t, kc(h, "checkout", "--workspace", "notes", "--to", dest), "already checked out")

	synced := asMap(t, body(t, kc(h, "sync", "--workspace", "notes", "--to", dest)))
	if synced["workspaceId"] != "notes" {
		t.Fatal(synced)
	}

	resolved := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	if resolved["pinId"] == nil || resolved["pinId"] == "" {
		t.Fatalf("resolve must emit pinId: %#v", resolved)
	}
}

func TestLoomCheckoutAsSkipsDeniedMount(t *testing.T) {
	t.Skip("writable Git worktree checkout retired with FileGit; kcfs is the host projection")
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	body(t, kc(h, "repo-add", "--repo", semantic))
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", "notes"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", alice))

	dest := filepath.Join(t.TempDir(), "agent")
	out := asMap(t, body(t, kc(h, "checkout", "--as", "bot", "--workspace", "notes", "--to", dest)))
	var skipped bool
	for _, raw := range out["mounts"].([]any) {
		m := asMap(t, raw)
		if m["repository"] == semantic {
			if m["skipped"] != true {
				t.Fatalf("semantic must be skipped for bot: %#v", out)
			}
			skipped = true
		}
	}
	if !skipped {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(dest, "refs", "semantic")); !os.IsNotExist(err) {
		t.Fatal("denied mount must not land on disk")
	}
}

func TestLoomRepoAddDirDoesNotStampExternalGit(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	alice := "kr://acme/personals/alice"
	failed := kc(h, "repo-add", "--repo", alice, "--driver", "filegit")
	expectCode(t, failed, "USAGE_INVALID")
	expectMsg(t, failed, "no longer supported")
}

func TestLoomPinReplayFreezesCommits(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", core))
	body(t, kc(h, "put", "--command-id", "v1", "--repo", core, "--object", "policy/A", "--value", `{"body":"first"}`))
	body(t, kc(h, "define-workspace", "--workspace", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	pin := asMap(t, body(t, kc(h, "resolve", "--workspace", "agent")))
	raw, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	pinFile := filepath.Join(t.TempDir(), "pin.json")
	if err := os.WriteFile(pinFile, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A", "--value", `{"body":"later"}`))
	live := body(t, kc(h, "read", "--workspace", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, live[0])["value"])["body"] != "later" {
		t.Fatal(live)
	}
	frozen := body(t, kc(h, "read", "--workspace", "agent", "--object", "policy/A", "--pin", pinFile)).([]any)
	if asMap(t, asMap(t, frozen[0])["value"])["body"] != "first" {
		t.Fatalf("replayed pin must not follow the live branch: %#v", frozen)
	}
}

func TestLoomDumpStateHidesDeniedRepos(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/core"
	secret := "kr://acme/restricted/classif"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", pub))
	body(t, kc(h, "repo-add", "--repo", secret))
	body(t, kc(h, "define-workspace", "--workspace", "company", "--revision", "1", "--source", pub+"=refs/heads/main"))
	body(t, kc(h, "define-workspace", "--workspace", "classif", "--revision", "1", "--source", secret+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "allow", "--principal", "bot", "--cmd", "read", "--repo", pub))

	state := asMap(t, body(t, kc(h, "read", "--catalog", "--as", "bot")))
	repos, _ := state["repositories"].([]any)
	if len(repos) != 1 || repos[0] != pub {
		t.Fatalf("bot must not see the secret repo: %#v", state)
	}
	var sawClassif bool
	for _, raw := range state["workspaces"].([]any) {
		if asMap(t, raw)["workspaceId"] == "classif" {
			sawClassif = true
		}
	}
	if sawClassif {
		t.Fatalf("a workspace whose every source is hidden must itself be hidden: %#v", state)
	}
}

func TestLoomRecipeTravelsWithAuthoritySnapshot(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", alice))
	body(t, kc(h, "repo-add", semantic))
	defined := asMap(t, body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	)))
	if defined["recipeFile"] != ".kc-workspace.yaml" || defined["recipeLocation"] != "repository" {
		t.Fatalf("define-workspace must commit the hitchhiking file: %#v", defined)
	}
	opened, err := cli.Open(h)
	if err != nil {
		t.Fatal(err)
	}
	repo, ok := opened.Store.Get(kernel.RepositoryID(alice))
	if !ok {
		opened.Close()
		t.Fatal("root authority is not attached")
	}
	tree, ok := snapshot.TreeStoreOf(repo)
	if !ok {
		opened.Close()
		t.Fatal("root authority has no TreeStore")
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		opened.Close()
		t.Fatal(err)
	}
	recipe, err := tree.ReadFile(".kc-workspace.yaml", head)
	opened.Close()
	if err != nil || !strings.Contains(string(recipe), "name: notes") {
		t.Fatalf("recipe was not persisted in authority snapshot: %q %v", recipe, err)
	}

	aliceDir := filepath.Join(h, "repos", cli.EncodeRepoDir(alice))
	bob := testkit.TempDir(t)
	body(t, kc(bob, "init", "--catalog", "kr://bob/catalog"))
	body(t, kc(bob, "repo-add", alice, "--dir", aliceDir))
	body(t, kc(bob, "repo-add", semantic))
	body(t, kc(bob, "define-workspace", "--from-repo", alice))
	pin := asMap(t, body(t, kc(bob, "resolve", "--workspace", "notes")))
	if pin["workspaceId"] != "notes" || len(asMap(t, pin["repositories"])) != 2 {
		t.Fatalf("attached authority must carry the recipe without redefining it: %#v", pin)
	}
}

func TestLoomDefineWorkspaceFromFile(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", alice))
	file := filepath.Join(t.TempDir(), ".kc-workspace.yaml")
	if err := os.WriteFile(file, []byte("name: notes\nmounts:\n  - repository: "+alice+"\n    selector: refs/heads/main\n    path: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defined := asMap(t, body(t, kc(h, "define-workspace", "--file", file)))
	if defined["workspaceId"] != "notes" {
		t.Fatal(defined)
	}
	pin := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	if pin["workspaceId"] != "notes" || asMap(t, pin["repositories"])[alice] == nil {
		t.Fatalf("file-defined recipe must resolve from the authority: %#v", pin)
	}
}

func TestMountPositionalRepoId(t *testing.T) {
	parsed, err := cli.ParseArgs([]string{"repo-add", "kr://acme/personals/alice", "--dir", "/tmp/alice-notes"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "repo-add" || len(parsed.Args) != 1 || parsed.Args[0] != "kr://acme/personals/alice" {
		t.Fatalf("%#v", parsed)
	}
	if cli.FlagString(parsed.Flags, "dir") != "/tmp/alice-notes" {
		t.Fatal(parsed.Flags)
	}
}

func TestLoomOverlayAndBaseRev(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	scratch := "kr://acme/personals/scratch"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", alice))
	body(t, kc(h, "repo-add", semantic))
	body(t, kc(h, "repo-add", scratch))
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	))

	overFile := filepath.Join(t.TempDir(), "overlay.yaml")
	if err := os.WriteFile(overFile, []byte(`
name: notes
mounts:
  - repository: kr://acme/personals/scratch
    selector: refs/heads/main
    path: scratch
`), 0o644); err != nil {
		t.Fatal(err)
	}
	overlaid := asMap(t, body(t, kc(h, "overlay", "--workspace", "notes", "--file", overFile)))
	sources, _ := overlaid["sources"].([]any)
	if len(sources) != 3 {
		t.Fatalf("overlay must add scratch: %#v", overlaid)
	}
	pin := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	repos, _ := pin["repositories"].(map[string]any)
	if _, ok := repos[scratch]; !ok || len(repos) != 3 {
		t.Fatalf("resolve must see overlay mounts: %#v", pin)
	}
	state := asMap(t, body(t, kc(h, "read", "--catalog")))
	var notes map[string]any
	for _, raw := range state["workspaces"].([]any) {
		item := asMap(t, raw)
		if item["workspaceId"] == "notes" {
			notes = item
		}
	}
	shared, _ := notes["sources"].([]any)
	if len(shared) != 2 {
		t.Fatalf("overlay must not rewrite the shared recipe: %#v", notes)
	}

	body(t, kc(h, "overlay", "--workspace", "notes", "--clear"))
	cleared := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	clearedRepos, _ := cleared["repositories"].(map[string]any)
	if _, ok := clearedRepos[scratch]; ok || len(clearedRepos) != 2 {
		t.Fatalf("clear must drop the overlay: %#v", cleared)
	}

	lockedPin := asMap(t, body(t, kc(h, "resolve", "--workspace", "notes")))
	aliceCommit, _ := asMap(t, lockedPin["repositories"])[alice].(string)
	body(t, kc(h, "define-workspace", "--workspace", "notes", "--revision", "2",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
		"--base-rev", alice+"="+aliceCommit,
	))
	body(t, kc(h, "put", "--command-id", "move-alice", "--repo", alice,
		"--object", "note/x", "--value", `{"text":"moved"}`))
	expectCode(t, kc(h, "resolve", "--workspace", "notes"), "NON_FAST_FORWARD")
}

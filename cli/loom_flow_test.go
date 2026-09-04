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
	seedRepo(t, h, core)
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

func TestCatalogInventoryDoesNotHideReposWithoutKnowledgeRead(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/core"
	secret := "kr://acme/restricted/classif"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, pub)
	seedRepo(t, h, secret)
	body(t, kc(h, "put", "--command-id", "secret-body", "--repo", secret,
		"--object", "policy/secret", "--value", `{"body":"classified"}`))
	body(t, kc(h, "define-workspace", "--workspace", "company", "--revision", "1", "--source", pub+"=refs/heads/main"))
	body(t, kc(h, "define-workspace", "--workspace", "classif", "--revision", "1", "--source", secret+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", "kr://acme/catalog"))

	state := asMap(t, body(t, kc(h, "read", "--catalog", "--as", "bot")))
	repos := businessRepositories(state)
	seen := map[string]bool{}
	for _, id := range repos {
		seen[id.(string)] = true
	}
	if !seen[pub] || !seen[secret] || len(repos) != 2 {
		t.Fatalf("catalog.read must discover every registered repository: %#v", state)
	}
	workspaceIDs := map[string]bool{}
	for _, raw := range state["workspaces"].([]any) {
		workspaceIDs[asMap(t, raw)["workspaceId"].(string)] = true
	}
	if !workspaceIDs["company"] || !workspaceIDs["classif"] {
		t.Fatalf("catalog.read must list named knowledge sets: %#v", state)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--repo", secret, "--object", "policy/secret"), "FORBIDDEN")
}

func TestLoomRecipeTravelsWithAuthoritySnapshot(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, alice)
	seedRepo(t, h, semantic)
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
	seedRepo(t, bob, alice, "--dir", aliceDir)
	seedRepo(t, bob, semantic)
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
	seedRepo(t, h, alice)
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
	seedRepo(t, h, alice)
	seedRepo(t, h, semantic)
	seedRepo(t, h, scratch)
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
	shared, _ := notes["repositories"].([]any)
	if len(shared) != 2 {
		t.Fatalf("overlay must not rewrite the shared recipe: %#v", notes)
	}
	if _, ok := notes["sources"]; ok {
		t.Fatalf("catalog inventory must not expose sources: %#v", notes)
	}
	for _, raw := range shared {
		if raw == scratch {
			t.Fatalf("shared recipe listed overlay-only repository: %#v", notes)
		}
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

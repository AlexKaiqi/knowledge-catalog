package cli_test

import (
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

func TestLoomPublishedDatasetFreezesUntilRepublish(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, core)
	body(t, kc(h, "put", "--command-id", "v1", "--repo", core, "--object", "policy/A", "--value", `{"body":"first"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", core+"=refs/heads/main"))
	first := asMap(t, body(t, kc(h, "read", "--dataset", "agent", "--object", "policy/A")).([]any)[0])
	c1 := first["commit"].(string)
	body(t, kc(h, "put", "--command-id", "v2", "--repo", core, "--object", "policy/A", "--value", `{"body":"later"}`))
	namedRead := body(t, kc(h, "read", "--dataset", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, namedRead[0])["value"])["body"] != "first" {
		t.Fatalf("named dataset latest must stay frozen until republish: %#v", namedRead)
	}
	frozen := asMap(t, body(t, kc(h, "read", "--repo", core, "--commit", c1, "--object", "policy/A")))
	if asMap(t, frozen["value"])["body"] != "first" {
		t.Fatalf("exact --repo --commit replay must not follow the live branch: %#v", frozen)
	}
	expectMsg(t, kc(h, "read", "--pin", "{}", "--object", "policy/A"), "rejects --pin")
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "2", "--source", core+"=refs/heads/main"))
	live := body(t, kc(h, "read", "--dataset", "agent", "--object", "policy/A")).([]any)
	if asMap(t, asMap(t, live[0])["value"])["body"] != "later" {
		t.Fatal(live)
	}
	replayed := asMap(t, body(t, kc(h, "read", "--repo", core, "--commit", c1, "--object", "policy/A")))
	if asMap(t, replayed["value"])["body"] != "first" {
		t.Fatalf("historical --repo --commit must keep the earlier Snapshot: %#v", replayed)
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
	body(t, kc(h, "dataset", "define", "--dataset", "company", "--revision", "1", "--source", pub+"=refs/heads/main"))
	body(t, kc(h, "dataset", "define", "--dataset", "classif", "--revision", "1", "--source", secret+"=refs/heads/main"))
	body(t, kc(h, "allow", "--principal", "bot", "--action", "catalog.read", "--catalog", "kr://acme/catalog"))

	state := asMap(t, body(t, kc(h, "show", "--as", "bot")))
	repos := businessRepositories(state)
	seen := map[string]bool{}
	for _, id := range repos {
		seen[id.(string)] = true
	}
	if !seen[pub] || !seen[secret] || len(repos) != 2 {
		t.Fatalf("catalog.read must discover every registered repository: %#v", state)
	}
	setIDs := map[string]bool{}
	for _, raw := range state["datasets"].([]any) {
		setIDs[asMap(t, raw)["id"].(string)] = true
	}
	if !setIDs["company"] || !setIDs["classif"] {
		t.Fatalf("catalog.read must list named knowledge sets: %#v", state)
	}
	expectCode(t, kc(h, "read", "--as", "bot", "--repo", secret, "--object", "policy/secret"), "FORBIDDEN")
}

func TestLoomRecipeTravelsWithAuthoritySnapshot(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	semantic := "kr://acme/public/semantic"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	aliceDSN := lakeFSRepoDSN(t)
	seedRepo(t, h, alice, "--dsn", aliceDSN)
	seedRepo(t, h, semantic)
	body(t, kc(h, "put", "--command-id", "alice-note", "--repo", alice, "--object", "note/x", "--value", `{"text":"seed"}`))
	defined := asMap(t, body(t, kc(h, "dataset", "define", "--dataset", "notes", "--revision", "1",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	)))
	if defined["recipeFile"] != ".kc-dataset.yaml" || defined["recipeLocation"] != "repository" {
		t.Fatalf("define-dataset must commit the hitchhiking file: %#v", defined)
	}
	published := body(t, kc(h, "read", "--dataset", "notes", "--object", "note/x")).([]any)
	if len(published) != 1 || asMap(t, published[0])["commit"] != defined["recipeCommit"] {
		t.Fatalf("dataset must freeze the hitchhiking commit, not the pre-recipe HEAD: read=%#v defined=%#v", published, defined)
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
	recipe, err := tree.ReadFile(".kc-dataset.yaml", head)
	opened.Close()
	if err != nil || !strings.Contains(string(recipe), "name: notes") {
		t.Fatalf("recipe was not persisted in authority snapshot: %q %v", recipe, err)
	}

	bob := testkit.TempDir(t)
	body(t, kc(bob, "init", "--catalog", "kr://bob/catalog"))
	seedRepo(t, bob, alice, "--dsn", aliceDSN)
	seedRepo(t, bob, semantic)
	body(t, kc(bob, "dataset", "define", "--from-repo", alice))
	state := asMap(t, body(t, kc(bob, "show")))
	var notes map[string]any
	for _, raw := range state["datasets"].([]any) {
		item := asMap(t, raw)
		if item["id"] == "notes" {
			notes = item
		}
	}
	if notes["id"] != "notes" || len(notes["repositories"].([]any)) != 2 {
		t.Fatalf("attached authority must carry the recipe without redefining it: %#v", state)
	}
	got := body(t, kc(bob, "read", "--dataset", "notes", "--object", "note/x")).([]any)
	if len(got) != 1 {
		t.Fatalf("cloned recipe must resolve listed files: %#v", got)
	}
}

func TestLoomDefineKnowledgeSetFromFile(t *testing.T) {
	h := testkit.TempDir(t)
	alice := "kr://acme/personals/alice"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	seedRepo(t, h, alice)
	body(t, kc(h, "put", "--command-id", "alice-note", "--repo", alice, "--object", "note/x", "--value", `{"text":"seed"}`))
	file := filepath.Join(t.TempDir(), ".kc-dataset.yaml")
	if err := os.WriteFile(file, []byte("name: notes\nmounts:\n  - repository: "+alice+"\n    selector: refs/heads/main\n    path: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defined := asMap(t, body(t, kc(h, "dataset", "define", "--file", file)))
	if defined["setId"] != "notes" {
		t.Fatal(defined)
	}
	got := body(t, kc(h, "read", "--dataset", "notes", "--object", "note/x")).([]any)
	if len(got) != 1 {
		t.Fatalf("file-defined recipe must resolve from the authority: %#v", got)
	}
}

func TestMountPositionalRepoId(t *testing.T) {
	parsed, err := cli.ParseArgs([]string{"attach", "kr://acme/personals/alice"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "attach" || len(parsed.Args) != 1 || parsed.Args[0] != "kr://acme/personals/alice" {
		t.Fatalf("%#v", parsed)
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
	body(t, kc(h, "put", "--command-id", "alice-note", "--repo", alice, "--object", "note/x", "--value", `{"text":"seed"}`))
	body(t, kc(h, "put", "--command-id", "scratch-note", "--repo", scratch, "--object", "note/scratch", "--value", `{"text":"overlay"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "notes", "--revision", "1",
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
	overlaid := asMap(t, body(t, kc(h, "overlay", "--dataset", "notes", "--file", overFile)))
	sources, _ := overlaid["sources"].([]any)
	if len(sources) != 3 {
		t.Fatalf("overlay must add scratch: %#v", overlaid)
	}
	scratchRead := body(t, kc(h, "read", "--dataset", "notes", "--object", "note/scratch")).([]any)
	if len(scratchRead) != 1 {
		t.Fatalf("resolve must see overlay mounts: %#v", scratchRead)
	}
	state := asMap(t, body(t, kc(h, "catalog-show")))
	var notes map[string]any
	for _, raw := range state["datasets"].([]any) {
		item := asMap(t, raw)
		if item["id"] == "notes" {
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

	body(t, kc(h, "overlay", "--dataset", "notes", "--clear"))
	cleared := body(t, kc(h, "read", "--dataset", "notes", "--object", "note/scratch")).([]any)
	if len(cleared) != 0 {
		t.Fatalf("clear must drop the overlay: %#v", cleared)
	}

	body(t, kc(h, "dataset", "define", "--dataset", "notes", "--revision", "2",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
	))
	republished := asMap(t, body(t, kc(h, "read", "--dataset", "notes", "--object", "note/x")).([]any)[0])
	aliceFrozen, _ := republished["commit"].(string)
	if aliceFrozen == "" {
		t.Fatalf("republish must freeze a commit: %#v", republished)
	}
	body(t, kc(h, "put", "--command-id", "move-alice", "--repo", alice,
		"--object", "note/x", "--value", `{"text":"moved"}`))
	stillFrozen := asMap(t, body(t, kc(h, "read", "--dataset", "notes", "--object", "note/x")).([]any)[0])
	if stillFrozen["commit"] != aliceFrozen || asMap(t, stillFrozen["value"])["text"] != "seed" {
		t.Fatalf("published dataset must keep the frozen commit: %#v", stillFrozen)
	}
	expectCode(t, kc(h, "dataset", "define", "--dataset", "notes", "--revision", "3",
		"--source", alice+"=refs/heads/main@",
		"--source", semantic+"=refs/heads/main@refs/semantic",
		"--base-rev", alice+"="+aliceFrozen,
	), "NON_FAST_FORWARD")
}

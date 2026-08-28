package dolt_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dolt"
)

func TestNativeDoltRepositoryContract(t *testing.T) {
	requireDoltRuntime(t)
	factory := func(t *testing.T, id string) snapshot.Store {
		t.Helper()
		dir := testkit.TempDir(t)
		repo, err := dolt.OpenDolt(dir, kernel.RepositoryID(id))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".dolt")); err != nil {
			t.Fatalf("native .dolt metadata missing: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
			t.Fatalf("Dolt authority must not be Git: %v", err)
		}
		return repo
	}
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}

func requireDoltRuntime(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("native Dolt adapter test is outside the short suite")
	}
	if _, err := exec.LookPath("dolt"); err == nil {
		return
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("live Dolt adapter is required: neither dolt nor docker is available")
		}
		t.Skip("neither dolt nor docker is available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, docker, "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatalf("live Dolt adapter is required but Docker daemon is unavailable: %v", err)
		}
		t.Skipf("dolt CLI is absent and Docker daemon is unavailable: %v", err)
	}
}

func TestNativeDoltRawFilesArePinnedAndCASProtected(t *testing.T) {
	requireDoltRuntime(t)
	repoAny, err := dolt.OpenDolt(testkit.TempDir(t), "kr://conformance/dolt-raw")
	if err != nil {
		t.Fatal(err)
	}
	repo := repoAny
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	v1, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "docs/state.txt", Content: []byte("V1\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: v1, ExpectedTargetCommit: v1,
		Changes: []snapshot.TreeChange{{Path: "docs/state.txt", Content: []byte("V2\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	old, err := repo.ReadFile("docs/state.txt", v1)
	if err != nil || string(old) != "V1\n" {
		t.Fatalf("old AS OF read = %q, %v", old, err)
	}
	live, err := repo.ReadFile("docs/state.txt", v2)
	if err != nil || string(live) != "V2\n" {
		t.Fatalf("live AS OF read = %q, %v", live, err)
	}
	_, err = repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: v1, ExpectedTargetCommit: v1,
		Changes: []snapshot.TreeChange{{Path: "docs/stale.txt", Content: []byte("no")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

func TestNativeDoltDirectoryReaderPagesOnlyDirectChildren(t *testing.T) {
	requireDoltRuntime(t)
	repo, err := dolt.OpenDolt(testkit.TempDir(t), "kr://conformance/dolt-directory")
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{
			{Path: "docs/a.txt", Content: []byte("a")},
			{Path: "docs/b.txt", Content: []byte("b")},
			{Path: "docs/nested/c.txt", Content: []byte("c")},
			{Path: "other.txt", Content: []byte("other")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.ReadDirectory(snapshot.DirectoryRequest{Commit: commit, Directory: "docs", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 2 || first.Exhausted || first.Continuation == "" {
		t.Fatalf("first page: %#v", first)
	}
	second, err := repo.ReadDirectory(snapshot.DirectoryRequest{
		Commit: commit, Directory: "docs", Limit: 2, Continuation: first.Continuation,
	})
	if err != nil {
		t.Fatal(err)
	}
	all := append(append([]snapshot.DirectoryEntry{}, first.Entries...), second.Entries...)
	want := []snapshot.DirectoryEntry{{Name: "a.txt", Kind: "file"}, {Name: "b.txt", Kind: "file"}, {Name: "nested", Kind: "directory"}}
	if len(all) != len(want) {
		t.Fatalf("direct children: %#v", all)
	}
	for i := range want {
		if all[i] != want[i] {
			t.Fatalf("direct children: got %#v want %#v", all, want)
		}
	}
	if !second.Exhausted || second.Continuation != "" || second.Generation != string(commit) {
		t.Fatalf("second page: %#v", second)
	}
	if _, err := repo.ReadDirectory(snapshot.DirectoryRequest{
		Commit: root, Directory: "docs", Limit: 2, Continuation: first.Continuation,
	}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("continuation basis mismatch: %v", err)
	}
}

func TestNativeDoltArchivePersistsAndBlocksBothWriteSurfaces(t *testing.T) {
	requireDoltRuntime(t)
	repoAny, err := dolt.OpenDolt(testkit.TempDir(t), "kr://conformance/dolt-archive")
	if err != nil {
		t.Fatal(err)
	}
	repo := repoAny
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	if err := repo.Archive(); err != nil {
		t.Fatal(err)
	}
	if !repo.Archived() {
		t.Fatal("archive ref is not visible")
	}
	_, err = testkit.OpenRepository(t, repo).ApplyKnowledgeCommit(testkit.CommitChange(repo.ID(), root, "blocked", 1, ""))
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
	_, err = repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "blocked.txt", Content: []byte("no")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
}

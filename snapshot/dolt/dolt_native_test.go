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
	"kc/knowledge"
	"kc/snapshot"
	"kc/snapshot/dolt"
)

func TestNativeDoltRepositoryContract(t *testing.T) {
	requireDoltRuntime(t)
	factory := func(t *testing.T, id string) knowledge.Repository {
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
	if _, err := exec.LookPath("dolt"); err == nil {
		return
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("neither dolt nor docker is available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, docker, "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
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
	_, err = repo.ApplyKnowledgeCommit(testkit.CommitChange(repo.ID(), root, "blocked", 1, ""))
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
	_, err = repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "blocked.txt", Content: []byte("no")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
}

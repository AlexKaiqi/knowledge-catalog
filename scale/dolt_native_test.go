package scale_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
	"kc/scale"
)

func TestNativeDoltRepositoryContract(t *testing.T) {
	if _, err := exec.LookPath("dolt"); err != nil {
		if _, dockerErr := exec.LookPath("docker"); dockerErr != nil {
			t.Skip("neither dolt nor docker is available")
		}
	}
	testkit.RepositoryContract(t, func(t *testing.T, id string) repository.Repository {
		t.Helper()
		dir := testkit.TempDir(t)
		repo, err := scale.OpenDolt(dir, kernel.RepositoryID(id))
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
	})
}

func TestNativeDoltRawFilesArePinnedAndCASProtected(t *testing.T) {
	repoAny, err := scale.OpenDolt(testkit.TempDir(t), "kr://conformance/dolt-raw")
	if err != nil {
		t.Fatal(err)
	}
	repo := repoAny.(*scale.DoltRepository)
	root := testkit.MustHead(t, repo, repository.DefaultRef)
	v1, err := repo.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: repository.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "docs/state.txt", Content: []byte("V1\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := repo.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: repository.DefaultRef,
		BaseCommit: v1, ExpectedTargetCommit: v1,
		Changes: []repository.RawFileChange{{Path: "docs/state.txt", Content: []byte("V2\n")}},
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
	_, err = repo.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: repository.DefaultRef,
		BaseCommit: v1, ExpectedTargetCommit: v1,
		Changes: []repository.RawFileChange{{Path: "docs/stale.txt", Content: []byte("no")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

func TestNativeDoltArchivePersistsAndBlocksBothWriteSurfaces(t *testing.T) {
	repoAny, err := scale.OpenDolt(testkit.TempDir(t), "kr://conformance/dolt-archive")
	if err != nil {
		t.Fatal(err)
	}
	repo := repoAny.(*scale.DoltRepository)
	root := testkit.MustHead(t, repo, repository.DefaultRef)
	if err := repo.Archive(); err != nil {
		t.Fatal(err)
	}
	if !repo.Archived() {
		t.Fatal("archive ref is not visible")
	}
	_, err = repo.ApplyCommit(testkit.CommitChange(repo.ID(), root, "blocked", 1, ""))
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
	_, err = repo.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: repository.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "blocked.txt", Content: []byte("no")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrRepositoryArchived)
}

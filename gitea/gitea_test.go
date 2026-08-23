package gitea_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"kc/gitea"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

func TestT12GiteaContract(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)
	factory := func(t *testing.T, id string) repository.Repository {
		t.Helper()
		sum := sha256.Sum256([]byte(id + run))
		name := "kc-" + hex.EncodeToString(sum[:8])
		dsn := base + "/kc/" + name
		repo, err := gitea.Open(kernel.RepositoryID(id), dsn, token)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	// SnapshotStore + Knowledge surface: see testkit.RepositoryContract.
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}

func TestGiteaReadPinnedCommitNotWorktree(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)
	id := kernel.RepositoryID("kr://conformance/gitea/pin")
	sum := sha256.Sum256([]byte(string(id) + run + "pin"))
	dsn := base + "/kc/kc-" + hex.EncodeToString(sum[:8])
	repo, err := gitea.Open(id, dsn, token)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, repo, "refs/heads/main")
	first, err := repo.ApplyCommit(testkit.CommitChange(id, root, "policy/P-1", map[string]any{"version": 1}, "policies/P-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ApplyCommit(testkit.CommitChange(id, first, "policy/P-1", map[string]any{"version": 2}, "policies/P-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	v1, err := repo.Read("policy/P-1", first)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := repo.Read("policy/P-1", second)
	if err != nil {
		t.Fatal(err)
	}
	if testkitAsInt(v1.Value.(map[string]any)["version"]) != 1 {
		t.Fatalf("pinned first %#v", v1.Value)
	}
	if testkitAsInt(v2.Value.(map[string]any)["version"]) != 2 {
		t.Fatalf("live second %#v", v2.Value)
	}
	if _, ok := any(repo).(interface{ RootDir() string }); ok {
		t.Fatal("gitea adapter must not expose a worktree RootDir")
	}
}

func testkitAsInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return -1
	}
}

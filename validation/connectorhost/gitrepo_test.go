package connectorhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHostSynchronizesOnlyCommittedPublicRepositoryState(t *testing.T) {
	ctx := context.Background()
	author := copyTestRepo(t)
	remote := filepath.Join(t.TempDir(), "public-connectors.git")
	mustGit(t, ctx, "init", "--bare", remote)
	mustGit(t, ctx, "-C", author, "init", "-b", "main")
	mustGit(t, ctx, "-C", author, "add", ".")
	mustGit(t, ctx, "-C", author, "-c", "user.name=connector-test", "-c", "user.email=connector-test@example.invalid", "commit", "-m", "initial connector")
	mustGit(t, ctx, "-C", author, "remote", "add", "origin", remote)
	mustGit(t, ctx, "-C", author, "push", "-u", "origin", "main")

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := HostConfig{
		Repository: remote, Ref: "refs/heads/main", RepoPath: filepath.Join(store.Home(), "repository"),
		SyncEvery: "1s", KCURL: "http://kc.invalid",
	}
	if err := store.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	host := NewHost(store, config, KCClient{BaseURL: config.KCURL})
	first := host.Sync(ctx)
	if first.Error != "" || first.Commit == "" {
		t.Fatalf("initial sync: %#v", first)
	}
	loaded, err := host.Connector("file-observer")
	if err != nil {
		t.Fatal(err)
	}
	firstGeneration := loaded.Generation

	mainPath := filepath.Join(author, "connectors", "file-observer", "main.go")
	f, err := os.OpenFile(mainPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n// committed public repository update\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	unpublished := host.Sync(ctx)
	if unpublished.Error != "" || unpublished.Commit != first.Commit {
		t.Fatalf("uncommitted author state leaked into runtime: %#v", unpublished)
	}
	loaded, _ = host.Connector("file-observer")
	if loaded.Generation != firstGeneration {
		t.Fatal("runtime generation changed before public repository push")
	}

	mustGit(t, ctx, "-C", author, "add", "connectors/file-observer/main.go")
	mustGit(t, ctx, "-C", author, "-c", "user.name=connector-test", "-c", "user.email=connector-test@example.invalid", "commit", "-m", "update connector")
	mustGit(t, ctx, "-C", author, "push", "origin", "main")
	published := host.Sync(ctx)
	if published.Error != "" || published.Commit == first.Commit {
		t.Fatalf("published commit not synchronized: %#v", published)
	}
	loaded, err = host.Connector("file-observer")
	if err != nil || loaded.Generation == firstGeneration {
		t.Fatalf("published generation not discovered: %#v %v", loaded, err)
	}
	if loaded.Dir == filepath.Join(author, "connectors", "file-observer") {
		t.Fatal("Host executed the user's development checkout instead of its own read copy")
	}
	if err := os.Rename(remote, remote+".offline"); err != nil {
		t.Fatal(err)
	}
	failed := host.Sync(ctx)
	if failed.Error == "" || failed.Commit != published.Commit {
		t.Fatalf("failed sync lost the last runnable commit: %#v", failed)
	}
}

func mustGit(t *testing.T, ctx context.Context, args ...string) string {
	t.Helper()
	output, err := runGit(ctx, args...)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

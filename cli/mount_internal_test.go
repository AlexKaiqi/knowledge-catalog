package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPathWithMissingTailResolvesExistingAlias(t *testing.T) {
	base := t.TempDir()
	realHome := filepath.Join(base, "real-home")
	if err := os.Mkdir(realHome, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasHome := filepath.Join(base, "alias-home")
	if err := os.Symlink(realHome, aliasHome); err != nil {
		t.Fatal(err)
	}

	got := canonicalPathWithMissingTail(filepath.Join(aliasHome, "repos", "new-repo"))
	canonicalHome, err := filepath.EvalSymlinks(realHome)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalHome, "repos", "new-repo")
	if got != want {
		t.Fatalf("canonical missing path = %q, want %q", got, want)
	}
}

func TestManagedRepoDirAcceptsMissingDirectoryBelowAliasedRoot(t *testing.T) {
	base := t.TempDir()
	realHome := filepath.Join(base, "real-home")
	if err := os.Mkdir(realHome, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasHome := filepath.Join(base, "alias-home")
	if err := os.Symlink(realHome, aliasHome); err != nil {
		t.Fatal(err)
	}
	stores := DefaultStores()

	managed := filepath.Join(realHome, defaultReposDir, "new-repo")
	if !managedRepoDir(aliasHome, stores, managed) {
		t.Fatalf("missing repository below aliased managed root was classified external: %s", managed)
	}
	outside := filepath.Join(base, "outside", "new-repo")
	if managedRepoDir(aliasHome, stores, outside) {
		t.Fatalf("outside repository was classified managed: %s", outside)
	}
}

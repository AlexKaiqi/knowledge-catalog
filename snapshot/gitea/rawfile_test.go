package gitea_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/gitea"
)

// RawFileStore over the Gitea contents API round-trips exact bytes and lists
// every blob path, not just the knowledge-shaped ones scanAt indexes for the
// object_id model.
func TestGiteaRawFileStoreRoundTrip(t *testing.T) {
	base, token, run := testkit.GiteaEndpoint(t)
	t.Setenv(gitea.EnvToken, token)
	id := kernel.RepositoryID("kr://conformance/gitea/raw")
	sum := sha256.Sum256([]byte(string(id) + run + "raw"))
	repo, err := gitea.Open(id, base+"/kc/kc-"+hex.EncodeToString(sum[:8]), token)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := snapshot.TreeStoreOf(repo)
	if !ok {
		t.Fatal("gitea.Repository must implement RawFileStore")
	}
	root := testkit.MustHead(t, repo, "refs/heads/main")

	commit, err := raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: id, TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{
			{Path: "vfs/note.md", Content: []byte("draft content\n")},
		},
		Message: "vfs write",
	})
	if err != nil {
		t.Fatal(err)
	}
	if commit == root {
		t.Fatal("commit did not advance")
	}

	got, err := raw.ReadFile("vfs/note.md", commit)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "draft content\n" {
		t.Fatalf("round trip must be byte-exact, got %q", got)
	}

	files, err := raw.ListFiles(commit)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range files {
		if p == "vfs/note.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListFiles must include the new path: %v", files)
	}

	// CAS still applies.
	_, err = raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: id, TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "vfs/other.md", Content: []byte("x")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)

	// Remove deletes the path.
	removed, err := raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: id, TargetRef: "refs/heads/main",
		BaseCommit: commit, ExpectedTargetCommit: commit,
		Changes: []snapshot.TreeChange{{Path: "vfs/note.md", Remove: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ReadFile("vfs/note.md", removed); err == nil {
		t.Fatal("removed path must not read back")
	}
}

package local_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

// RawFileStore writes literal bytes at a literal path — no frontmatter, no
// object_id — and ReadFile round-trips them exactly, including whitespace
// ApplyCommit's Knowledge-shaped writes never had to preserve byte-for-byte.
func TestRawFileStoreRoundTripsExactBytes(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	raw, ok := repository.RawFileStoreOf(repo)
	if !ok {
		t.Fatal("FileGitRepository must implement RawFileStore")
	}
	root := testkit.MustHead(t, repo, "")
	commit, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{
			{Path: "notes/draft.md", Content: []byte("line one\nline two\n\n")},
		},
		Message: "vfs write",
	})
	if err != nil {
		t.Fatal(err)
	}
	if commit == root {
		t.Fatal("commit did not advance")
	}
	got, err := raw.ReadFile("notes/draft.md", commit)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "line one\nline two\n\n" {
		t.Fatalf("round trip must be byte-exact, got %q", got)
	}
	files, err := raw.ListFiles(commit)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range files {
		if p == "notes/draft.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListFiles must include the new path: %v", files)
	}
}

// A stale ExpectedTargetCommit is CAS-checked exactly like ApplyCommit's.
func TestRawFileStoreRejectsStaleBase(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	raw, _ := repository.RawFileStoreOf(repo)
	root := testkit.MustHead(t, repo, "")
	if _, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "a.txt", Content: []byte("a")}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "b.txt", Content: []byte("b")}},
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}

// A path attempting to escape the repository root is rejected, the same
// guard the Knowledge write path uses.
func TestRawFileStoreRejectsPathEscape(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	raw, _ := repository.RawFileStoreOf(repo)
	root := testkit.MustHead(t, repo, "")
	_, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "../escape.txt", Content: []byte("x")}},
	})
	if err == nil {
		t.Fatal("expected a path-escape refusal")
	}
}

// git show <rev>:<path> succeeds for a directory path too (it prints a tree
// listing as if it were content) — ReadFile must reject that, not hand back
// garbage that happens to decode as text.
func TestRawFileStoreRejectsReadingADirectory(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	raw, _ := repository.RawFileStoreOf(repo)
	root := testkit.MustHead(t, repo, "")
	commit, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "analysis/churn.md", Content: []byte("x")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ReadFile("analysis", commit); err == nil {
		t.Fatal("reading a directory path must fail, not return its tree listing as content")
	}
}

// Remove deletes the path; a subsequent read is missing, not empty.
func TestRawFileStoreRemove(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	raw, _ := repository.RawFileStoreOf(repo)
	root := testkit.MustHead(t, repo, "")
	first, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []repository.RawFileChange{{Path: "gone.txt", Content: []byte("x")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := raw.ApplyRawCommit(repository.RawFileChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: first, ExpectedTargetCommit: first,
		Changes: []repository.RawFileChange{{Path: "gone.txt", Remove: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ReadFile("gone.txt", second); err == nil {
		t.Fatal("removed path must not read back")
	}
}

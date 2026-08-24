package gitdir_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/gitdir"
)

func openDir(t *testing.T) *gitdir.Dir {
	t.Helper()
	d, err := gitdir.Open(filepath.Join(t.TempDir(), "tree"), "streams/")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func write(t *testing.T, d *gitdir.Dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d.Root(), name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenIsIdempotentAndLeavesRootCommit(t *testing.T) {
	d := openDir(t)
	head, ok := d.Rev("")
	if !ok || head == "" {
		t.Fatal("Open must leave a root commit")
	}
	again, err := gitdir.Open(d.Root(), "streams/")
	if err != nil {
		t.Fatal(err)
	}
	if second, _ := again.Rev(""); second != head {
		t.Fatalf("re-Open moved head: %s -> %s", head, second)
	}
	if !d.HasCommit(head) || d.HasCommit("deadbeef") {
		t.Fatal("HasCommit disagrees with rev-parse")
	}
}

func TestCommitWorktreeIsNoOpWhenClean(t *testing.T) {
	d := openDir(t)
	head, _ := d.Rev(gitdir.BranchRef(gitdir.DefaultBranch))
	got, err := d.CommitWorktree(head, gitdir.Signature{Message: "nothing"})
	if err != nil {
		t.Fatal(err)
	}
	if got != head {
		t.Fatalf("clean tree must not commit: %s != %s", got, head)
	}
}

func TestCommitWorktreeRefusesMovedRef(t *testing.T) {
	d := openDir(t)
	write(t, d, "a.yaml", "id: a\n")
	if _, err := d.CommitWorktree("", gitdir.Signature{Message: "add a"}); err != nil {
		t.Fatal(err)
	}
	write(t, d, "b.yaml", "id: b\n")
	_, err := d.CommitWorktree("0000000000000000000000000000000000000000", gitdir.Signature{Message: "add b"})
	moved, ok := err.(gitdir.ErrMoved)
	if !ok {
		t.Fatalf("want ErrMoved, got %v", err)
	}
	if moved.Ref != gitdir.BranchRef(gitdir.DefaultBranch) {
		t.Fatal(moved)
	}
}

func TestPathsAndShowReadCommittedTree(t *testing.T) {
	d := openDir(t)
	write(t, d, "catalog.yaml", "id: kr://acme/catalog\n")
	head, err := d.CommitWorktree("", gitdir.Signature{Message: "catalog: persist"})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := d.Paths(head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "catalog.yaml" {
		t.Fatal(paths)
	}
	body, err := d.Show(head, "catalog.yaml")
	if err != nil || body != "id: kr://acme/catalog" {
		t.Fatalf("Show %q %v", body, err)
	}
}

func TestExcludeKeepsPackingDirectoriesOutOfCommits(t *testing.T) {
	d := openDir(t)
	if err := os.MkdirAll(filepath.Join(d.Root(), "streams"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, d, "streams/events.jsonl", "{}\n")
	write(t, d, "kept.yaml", "id: kept\n")
	head, err := d.CommitWorktree("", gitdir.Signature{Message: "add"})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := d.Paths(head)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if p != "kept.yaml" {
			t.Fatalf("streams/ must stay untracked, got %v", paths)
		}
	}
}

func TestLogSplitsTrailers(t *testing.T) {
	d := openDir(t)
	write(t, d, "workspace-duty.yaml", "workspaceId: duty\n")
	if _, err := d.CommitWorktree("", gitdir.Signature{
		Author: "alice", Message: "define-workspace duty", RequestID: "req-7", RuleID: "rule-3",
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := d.Log(10, "workspace-duty.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal(entries)
	}
	got := entries[0]
	if got.Author != "alice" || got.Message != "define-workspace duty" {
		t.Fatal(got)
	}
	if got.RequestID != "req-7" || got.RuleID != "rule-3" {
		t.Fatal(got)
	}
}

func TestLogOnMissingPathIsEmptyNotError(t *testing.T) {
	d := openDir(t)
	entries, err := d.Log(10, "never-existed.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal(entries)
	}
}

func TestSignatureFormatAndParseRoundTrip(t *testing.T) {
	sig := gitdir.Signature{Author: "bob\nsmith", Message: " register ", RequestID: "r1", RuleID: "g2"}
	name, email, message := sig.Format()
	if name != "bob smith" || email != gitdir.DefaultEmail {
		t.Fatalf("name %q email %q", name, email)
	}
	requestID, ruleID := gitdir.ParseTrailers(message)
	if requestID != "r1" || ruleID != "g2" {
		t.Fatalf("trailers %q %q", requestID, ruleID)
	}
}

func TestGitErrorSurfacesStderrNotJustExitStatus(t *testing.T) {
	d := openDir(t)
	taken := filepath.Join(d.Root(), "taken")
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taken, "occupied"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	head, _ := d.Rev("")
	err := d.AddWorktree(taken, head)
	if err == nil {
		t.Fatal("worktree add onto an existing non-empty directory must fail")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error must carry git's own diagnostic, not just an exit code: %v", err)
	}
}

func TestAddWorktreeIsDetachedAndPinnedAtCommit(t *testing.T) {
	d := openDir(t)
	write(t, d, "a.yaml", "id: a\n")
	head, err := d.CommitWorktree("", gitdir.Signature{Message: "add a"})
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "linked")
	if err := d.AddWorktree(dest, head); err != nil {
		t.Fatal(err)
	}
	linked := gitdir.At(dest)
	if got, ok := linked.Rev(""); !ok || got != head {
		t.Fatalf("linked worktree must be pinned at head: %s ok=%v want %s", got, ok, head)
	}
	if ref := linked.CheckedOutRef(); ref != "" {
		t.Fatalf("a checkout mount must be detached, not on a branch: %q", ref)
	}
	// The whole point of a detached linked worktree: the main worktree can
	// still switch branches freely while this one exists (this is exactly
	// what local.FileGitRepository.ApplyCommit does on every write).
	if err := d.Checkout(gitdir.DefaultBranch); err != nil {
		t.Fatalf("main worktree must stay free to switch branches: %v", err)
	}
}

func TestRemoveWorktreeDiscardsIt(t *testing.T) {
	d := openDir(t)
	head, _ := d.Rev("")
	dest := filepath.Join(t.TempDir(), "linked")
	if err := d.AddWorktree(dest, head); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveWorktree(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("RemoveWorktree must delete the directory too: %v", err)
	}
}

func TestShowRawPreservesTrailingWhitespaceUnlikeShow(t *testing.T) {
	d := openDir(t)
	write(t, d, "raw.md", "line one\nline two\n\n")
	head, err := d.CommitWorktree("", gitdir.Signature{Message: "add raw"})
	if err != nil {
		t.Fatal(err)
	}
	trimmed, err := d.Show(head, "raw.md")
	if err != nil {
		t.Fatal(err)
	}
	if trimmed != "line one\nline two" {
		t.Fatalf("Show is documented to trim: %q", trimmed)
	}
	raw, err := d.ShowRaw(head, "raw.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "line one\nline two\n\n" {
		t.Fatalf("ShowRaw must round-trip exact bytes, got %q", raw)
	}
}

func TestObjectTypeDistinguishesBlobFromTree(t *testing.T) {
	d := openDir(t)
	if err := os.MkdirAll(filepath.Join(d.Root(), "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, d, "dir/file.md", "hi\n")
	head, err := d.CommitWorktree("", gitdir.Signature{Message: "add"})
	if err != nil {
		t.Fatal(err)
	}
	if kind, ok := d.ObjectType(head, "dir/file.md"); !ok || kind != "blob" {
		t.Fatalf("file must report blob: %q ok=%v", kind, ok)
	}
	if kind, ok := d.ObjectType(head, "dir"); !ok || kind != "tree" {
		t.Fatalf("directory must report tree, not blob: %q ok=%v", kind, ok)
	}
	if _, ok := d.ObjectType(head, "never-existed"); ok {
		t.Fatal("a missing path must report ok=false")
	}
}

func TestSignatureFormatFillsDefaults(t *testing.T) {
	name, _, message := gitdir.Signature{}.Format()
	if name != gitdir.DefaultAuthor || message != gitdir.DefaultMessage {
		t.Fatalf("name %q message %q", name, message)
	}
}

func TestPorcelainChangesReportsWritesAndDeletes(t *testing.T) {
	d := openDir(t)
	write(t, d, "keep.md", "keep\n")
	write(t, d, "gone.md", "gone\n")
	if _, err := d.CommitWorktree("", gitdir.Signature{Message: "seed"}); err != nil {
		t.Fatal(err)
	}
	write(t, d, "keep.md", "edited\n")
	if err := os.Remove(filepath.Join(d.Root(), "gone.md")); err != nil {
		t.Fatal(err)
	}
	write(t, d, "new.md", "new\n")
	changes, err := d.PorcelainChanges()
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]gitdir.WorktreeChange{}
	for _, ch := range changes {
		byPath[ch.Path] = ch
	}
	if !byPath["gone.md"].Removed {
		t.Fatalf("delete: %#v", changes)
	}
	if byPath["keep.md"].Removed || byPath["new.md"].Removed {
		t.Fatalf("writes must not look like deletes: %#v", changes)
	}
}

func TestSparseCheckoutHidesUnmountedSubtree(t *testing.T) {
	d := openDir(t)
	if err := os.MkdirAll(filepath.Join(d.Root(), "docs/knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, d, "docs/knowledge/a.md", "a\n")
	write(t, d, "other.md", "no\n")
	head, err := d.CommitWorktree("", gitdir.Signature{Message: "seed"})
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "wt")
	if err := d.AddWorktree(dest, head); err != nil {
		t.Fatal(err)
	}
	wt := gitdir.At(dest)
	if err := wt.SparseCheckout("docs/knowledge"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "docs/knowledge/a.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "other.md")); !os.IsNotExist(err) {
		t.Fatalf("sparse-checkout must hide paths outside the subPath, err=%v", err)
	}
}

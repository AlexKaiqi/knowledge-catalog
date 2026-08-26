package catalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// A single-member workspace mounted at root: the checkout is not a read-only
// export, it is that member's own git working tree, genuinely editable.
func TestCheckoutMountsProducesAWritableWorktreeAtRoot(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
	}); err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, repo, "refs/heads/main")

	dest := filepath.Join(testkit.TempDir(t), "work")
	mounts, err := cat.CheckoutMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].Dir != dest || mounts[0].Commit != head {
		t.Fatalf("unexpected mounts: %#v (want dir %s commit %s)", mounts, dest, head)
	}

	if err := os.WriteFile(filepath.Join(dest, "analysis.md"), []byte("draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err := catalog.MountStatus(mounts)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || !statuses[0].Dirty || len(statuses[0].Changed) != 1 {
		t.Fatalf("editing inside the checkout must be visible to git status: %#v", statuses)
	}
}

// A root mount plus a nested mount compose into one tree; the nested mount's
// own commit is independent of the root's, and the root's git status is kept
// quiet about it via info/exclude rather than by copying content.
func TestCheckoutMountsComposesRootAndNestedMount(t *testing.T) {
	store := snapshot.NewRegistry()
	alice := testkit.MakeRepository(t, "kr://acme/personals/alice")
	semantic := testkit.MakeRepository(t, "kr://acme/public/semantic")
	if err := store.Add(alice); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(semantic); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: alice.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: semantic.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(testkit.TempDir(t), "work")
	mounts, err := cat.CheckoutMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %#v", mounts)
	}
	nestedDir := filepath.Join(dest, "refs", "semantic")
	if _, err := os.Stat(nestedDir); err != nil {
		t.Fatalf("nested mount must land inside the root's tree: %v", err)
	}

	exclude, err := os.ReadFile(filepath.Join(alice.RootDir(), ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exclude), "/refs") {
		t.Fatalf("root member's exclude must hide the nested mount from its own status: %q", exclude)
	}
	if !strings.Contains(string(exclude), "/"+catalog.MountCheckoutPinFile) {
		t.Fatalf("root member's exclude must also hide the checkout pin file: %q", exclude)
	}
}

// A remote-only member (no local git directory) cannot become a writable
// worktree; CheckoutMounts must not fail the whole call over it, or a single
// gitea-backed mount would make an otherwise-local workspace uncheckoutable.
// It is reported Skipped, with a Reason naming the missing capability, and
// still gets a directory reserved for it under root.
func TestCheckoutMountsReportsCapabilityGapWithoutFailingTheWholeCheckout(t *testing.T) {
	store := snapshot.NewRegistry()
	writable := testkit.MakeRepository(t, "kr://acme/personals/alice")
	plain := plainSnapshot{Store: testkit.MakeRepository(t, "kr://acme/public/semantic")}
	if _, ok := snapshot.TreeStoreOf(plain); ok {
		t.Fatal("fixture must hide immutable tree access")
	}
	if err := store.Add(writable); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(plain); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: writable.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: plain.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(testkit.TempDir(t), "work")
	mounts, err := cat.CheckoutMounts("notes", dest)
	if err != nil {
		t.Fatalf("a capability gap on one mount must not fail the others: %v", err)
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mount reports, got %#v", mounts)
	}
	var writableOut, skippedOut catalog.MountCheckout
	for _, m := range mounts {
		if m.Repository == writable.ID() {
			writableOut = m
		} else {
			skippedOut = m
		}
	}
	if writableOut.Skipped || writableOut.Dir != dest {
		t.Fatalf("the local mount must still check out normally: %#v", writableOut)
	}
	if !skippedOut.Skipped || skippedOut.Dir != "" || !strings.Contains(skippedOut.Reason, "local git directory") {
		t.Fatalf("the remote-only mount must be reported Skipped with its reason: %#v", skippedOut)
	}
	if _, err := os.Stat(filepath.Join(dest, "refs", "semantic")); err != nil {
		t.Fatalf("a directory must still be reserved at the skipped mount's path: %v", err)
	}

	statuses, err := catalog.MountStatus(mounts)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if s.Repository == plain.ID() && !s.Skipped {
			t.Fatalf("MountStatus must carry Skipped through, not try to git-status an empty directory: %#v", s)
		}
	}
}

// A relative checkout root must land where the caller's cwd says, not inside
// whichever member repository AddWorktree happens to run its git command
// from — AddWorktree resolves a relative dest against the source repo's own
// directory (cmd.Dir), so CheckoutMounts must make root absolute itself.
func TestCheckoutMountsAcceptsARelativeRoot(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
	}); err != nil {
		t.Fatal(err)
	}

	cwd := testkit.TempDir(t)
	prior, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prior) })

	mounts, err := cat.CheckoutMounts("notes", "work")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, "work")
	if len(mounts) != 1 {
		t.Fatalf("relative root must resolve against cwd (%s), got %#v", want, mounts)
	}
	wantInfo, wantErr := os.Stat(want)
	gotInfo, gotErr := os.Stat(mounts[0].Dir)
	if wantErr != nil || gotErr != nil || !os.SameFile(wantInfo, gotInfo) {
		t.Fatalf("relative root must resolve against cwd (%s), got %#v", want, mounts)
	}
	if _, err := os.Stat(filepath.Join(repo.RootDir(), "work")); err == nil {
		t.Fatal("checkout must not land inside the member repository's own directory")
	}
}

// Checkout needs mount paths, not a pure federated-read recipe: it must refuse
// rather than guess a layout nobody declared.
func TestCheckoutMountsRequiresDeclaredPaths(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := cat.CheckoutMounts("v", filepath.Join(testkit.TempDir(t), "work"))
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestCheckoutMountsAllowingSkipsDeniedMountsWithoutTouchingDisk(t *testing.T) {
	store := snapshot.NewRegistry()
	alice := testkit.MakeRepository(t, "kr://acme/personals/alice")
	semantic := testkit.MakeRepository(t, "kr://acme/public/semantic")
	if err := store.Add(alice); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(semantic); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: alice.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: semantic.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(testkit.TempDir(t), "work")
	mounts, err := cat.CheckoutMountsAllowing("notes", dest, map[kernel.RepositoryID]string{
		semantic.ID(): "not allowed to read " + string(semantic.ID()),
	})
	if err != nil {
		t.Fatal(err)
	}
	var skipped catalog.MountCheckout
	for _, m := range mounts {
		if m.Repository == semantic.ID() {
			skipped = m
		}
	}
	if !skipped.Skipped || skipped.Dir != "" {
		t.Fatalf("denied mount must be skipped with no dir: %#v", mounts)
	}
	if _, err := os.Stat(filepath.Join(dest, "refs", "semantic")); !os.IsNotExist(err) {
		t.Fatal("a denied mount must not land on disk")
	}
}

func TestCollectMountChangesReadsDirtyWorktreeFiles(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(testkit.TempDir(t), "work")
	mounts, err := cat.CheckoutMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "analysis.md"), []byte("draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writes, err := catalog.CollectMountChanges(mounts)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || writes[0].Path != "analysis.md" || string(writes[0].Content) != "draft\n" || writes[0].Remove {
		t.Fatalf("unexpected writes: %#v", writes)
	}
}

func TestCheckoutMountsHonoursSubPathSparseCheckout(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/public/docs")
	head := testkit.MustHead(t, repo, "refs/heads/main")
	raw, ok := snapshot.TreeStoreOf(repo)
	if !ok {
		t.Fatal("fixture must support raw writes")
	}
	if _, err := raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{
			{Path: "docs/knowledge/a.md", Content: []byte("keep\n")},
			{Path: "other.md", Content: []byte("hide\n")},
		},
		Message: "seed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("kb", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("kb"), SubPath: "docs/knowledge"},
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(testkit.TempDir(t), "work")
	if _, err := cat.CheckoutMounts("kb", dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "kb", "docs", "knowledge", "a.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "kb", "other.md")); !os.IsNotExist(err) {
		t.Fatalf("SubPath must hide the rest of the member: %v", err)
	}
}

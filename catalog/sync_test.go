package catalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

// CheckoutMounts must leave a record of what it materialized, and refuse to
// run twice against the same root: the second call cannot tell "fresh
// checkout" from "someone is about to clobber an existing one" without it.
func TestCheckoutMountsWritesPinFileAndRefusesRepeat(t *testing.T) {
	store := repository.NewStore()
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
	if _, err := cat.CheckoutMounts("notes", dest); err != nil {
		t.Fatal(err)
	}

	pinPath := filepath.Join(dest, catalog.CheckoutPinFile)
	if _, err := os.Stat(pinPath); err != nil {
		t.Fatalf("checkout must leave %s: %v", catalog.CheckoutPinFile, err)
	}

	_, err := cat.CheckoutMounts("notes", dest)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !containsAll(got, "already checked out", "SyncMounts") {
		t.Fatalf("refusal must point at SyncMounts, got %v", err)
	}
}

// SyncMounts is the other half of that contract: it refuses a directory that
// was never checked out, rather than guessing a layout for it.
func TestSyncMountsRequiresPriorCheckout(t *testing.T) {
	store := repository.NewStore()
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
	_, err := cat.SyncMounts("notes", filepath.Join(testkit.TempDir(t), "never-checked-out"))
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if !containsAll(err.Error(), "never been checked out", "CheckoutMounts") {
		t.Fatalf("refusal must point at CheckoutMounts, got %v", err)
	}
}

// The central Sync guarantee: independent per mount. A mount with no local
// changes advances straight to the newly resolved commit; a mount with local
// changes is left exactly alone and reported Blocked, so nothing is lost.
func TestSyncMountsAdvancesCleanMountAndBlocksOnDirtyOne(t *testing.T) {
	store := repository.NewStore()
	clean := testkit.MakeRepository(t, "kr://acme/public/semantic")
	dirty := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := store.Add(clean); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(dirty); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: dirty.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: clean.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(testkit.TempDir(t), "work")
	mounts, err := cat.CheckoutMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	var firstDirty catalog.MountCheckout
	for _, m := range mounts {
		if m.Repository != clean.ID() {
			firstDirty = m
		}
	}

	// Advance both upstream repos past what was checked out.
	cleanBase := testkit.MustHead(t, clean, "refs/heads/main")
	cleanNext, err := clean.ApplyCommit(testkit.CommitChange(clean.ID(), cleanBase, "metric/wau", map[string]any{"v": 1}, ""))
	if err != nil {
		t.Fatal(err)
	}
	dirtyBase := testkit.MustHead(t, dirty, "refs/heads/main")
	dirtyNext, err := dirty.ApplyCommit(testkit.CommitChange(dirty.ID(), dirtyBase, "note/retention", map[string]any{"v": 1}, ""))
	if err != nil {
		t.Fatal(err)
	}

	// Make the root mount dirty; leave the nested one clean.
	if err := os.WriteFile(filepath.Join(firstDirty.Dir, "draft.md"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	syncs, err := cat.SyncMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	var syncedClean, syncedDirty catalog.MountSync
	for _, s := range syncs {
		if s.Repository == clean.ID() {
			syncedClean = s
		} else {
			syncedDirty = s
		}
	}
	if syncedClean.Outcome != catalog.SyncAdvanced || syncedClean.To != cleanNext {
		t.Fatalf("clean mount must advance to the new commit: %#v", syncedClean)
	}
	if syncedDirty.Outcome != catalog.SyncBlocked || syncedDirty.To != dirtyNext {
		t.Fatalf("dirty mount must be blocked but still report what it's waiting on: %#v", syncedDirty)
	}

	if got, _ := os.ReadFile(filepath.Join(firstDirty.Dir, "draft.md")); string(got) != "uncommitted\n" {
		t.Fatal("a blocked mount's uncommitted file must survive Sync untouched")
	}

	// The pin file must reflect reality: advanced for clean, still-old for blocked.
	pin, err := readPin(t, dest)
	if err != nil {
		t.Fatal(err)
	}
	byRepo := map[kernel.RepositoryID]catalog.MountCheckout{}
	for _, m := range pin.Mounts {
		byRepo[m.Repository] = m
	}
	if byRepo[clean.ID()].Commit != cleanNext {
		t.Fatalf("pin must record the advanced commit: %#v", pin)
	}
	if byRepo[dirty.ID()].Commit != firstDirty.Commit {
		t.Fatalf("pin must keep the old commit for the blocked mount: %#v", pin)
	}
}

// A no-op Sync (nothing moved upstream) reports Unchanged, not Advanced —
// SyncOutcome is a read of what happened, not just "did we try".
func TestSyncMountsReportsUnchangedWhenNothingMoved(t *testing.T) {
	store := repository.NewStore()
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
	if _, err := cat.CheckoutMounts("notes", dest); err != nil {
		t.Fatal(err)
	}
	syncs, err := cat.SyncMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(syncs) != 1 || syncs[0].Outcome != catalog.SyncUnchanged {
		t.Fatalf("nothing moved upstream, expected Unchanged: %#v", syncs)
	}
}

// A mount added to the recipe after the first checkout is materialized on
// the next Sync, reported CheckedOut, without disturbing the mounts that
// were already there.
func TestSyncMountsMaterializesAMountAddedAfterCheckout(t *testing.T) {
	store := repository.NewStore()
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
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(testkit.TempDir(t), "work")
	if _, err := cat.CheckoutMounts("notes", dest); err != nil {
		t.Fatal(err)
	}

	if _, err := cat.DefineWorkspace("notes", 2, []catalog.WorkspaceSource{
		{Repository: alice.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: semantic.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}

	syncs, err := cat.SyncMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	var added catalog.MountSync
	found := false
	for _, s := range syncs {
		if s.Repository == semantic.ID() {
			added, found = s, true
		}
	}
	if !found || added.Outcome != catalog.SyncCheckedOut {
		t.Fatalf("a newly declared mount must be checked out fresh, not errored on: %#v", syncs)
	}
	if _, err := os.Stat(filepath.Join(dest, "refs", "semantic")); err != nil {
		t.Fatalf("the new mount must actually land on disk: %v", err)
	}
}

// A mount's Path changing since the last checkout is a recipe shape change,
// not a version advance: Sync must refuse rather than guess how to move it.
func TestSyncMountsRejectsAPathChangeSinceCheckout(t *testing.T) {
	store := repository.NewStore()
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
	if _, err := cat.CheckoutMounts("notes", dest); err != nil {
		t.Fatal(err)
	}

	if _, err := cat.DefineWorkspace("notes", 2, []catalog.WorkspaceSource{
		{Repository: alice.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: semantic.ID(), Selector: "refs/heads/main", Path: catalog.MountPath("kb/semantic")},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := cat.SyncMounts("notes", dest)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if !containsAll(err.Error(), "moved from mount path", "re-checkout") {
		t.Fatalf("refusal must explain it is a recipe shape change: %v", err)
	}
}

func readPin(t *testing.T, dest string) (catalog.CheckoutPin, error) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dest, catalog.CheckoutPinFile))
	if err != nil {
		return catalog.CheckoutPin{}, err
	}
	var pin catalog.CheckoutPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		return catalog.CheckoutPin{}, err
	}
	return pin, nil
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

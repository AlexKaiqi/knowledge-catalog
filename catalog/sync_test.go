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
	"kc/snapshot"
)

// CheckoutMounts must leave a record of what it materialized, and refuse to
// run twice against the same root: the second call cannot tell "fresh
// checkout" from "someone is about to clobber an existing one" without it.
func TestCheckoutMountsWritesPinFileAndRefusesRepeat(t *testing.T) {
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
	if _, err := cat.CheckoutMounts("notes", dest); err != nil {
		t.Fatal(err)
	}

	pinPath := filepath.Join(dest, catalog.MountCheckoutPinFile)
	if _, err := os.Stat(pinPath); err != nil {
		t.Fatalf("checkout must leave %s: %v", catalog.MountCheckoutPinFile, err)
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
	_, err := cat.SyncMounts("notes", filepath.Join(testkit.TempDir(t), "never-checked-out"))
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if !containsAll(err.Error(), "never been checked out", "CheckoutMounts") {
		t.Fatalf("refusal must point at CheckoutMounts, got %v", err)
	}
}

// Formal authorities do not expose a writable Git worktree. Sync preserves
// that capability decision, reports it explicitly, and advances the pin to
// the newly resolved immutable commit without pretending files were checked out.
func TestSyncMountsPreservesCapabilitySkipAndAdvancesPin(t *testing.T) {
	store := snapshot.NewRegistry()
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
	for _, mount := range mounts {
		if !mount.Skipped || mount.Dir != "" {
			t.Fatalf("formal authority must be reported as a skipped writable checkout: %#v", mount)
		}
	}

	// Advance both upstream repos past what was checked out.
	cleanBase := testkit.MustHead(t, clean, "refs/heads/main")
	cleanNext, err := clean.ApplyKnowledgeCommit(testkit.CommitChange(clean.ID(), cleanBase, "metric/wau", map[string]any{"v": 1}, ""))
	if err != nil {
		t.Fatal(err)
	}
	dirtyBase := testkit.MustHead(t, dirty, "refs/heads/main")
	dirtyNext, err := dirty.ApplyKnowledgeCommit(testkit.CommitChange(dirty.ID(), dirtyBase, "note/retention", map[string]any{"v": 1}, ""))
	if err != nil {
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
	if syncedClean.Outcome != catalog.SyncSkipped || syncedClean.To != cleanNext {
		t.Fatalf("capability-skipped mount must report the new pin: %#v", syncedClean)
	}
	if syncedDirty.Outcome != catalog.SyncSkipped || syncedDirty.To != dirtyNext {
		t.Fatalf("capability-skipped mount must report the new pin: %#v", syncedDirty)
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
	if byRepo[dirty.ID()].Commit != dirtyNext {
		t.Fatalf("pin must record the newly resolved commit for a skipped mount: %#v", pin)
	}
}

// A no-op Sync (nothing moved upstream) reports Unchanged, not Advanced —
// SyncOutcome is a read of what happened, not just "did we try".
func TestSyncMountsReportsUnchangedWhenNothingMoved(t *testing.T) {
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
	if _, err := cat.CheckoutMounts("notes", dest); err != nil {
		t.Fatal(err)
	}
	syncs, err := cat.SyncMounts("notes", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(syncs) != 1 || syncs[0].Outcome != catalog.SyncSkipped {
		t.Fatalf("writable checkout remains explicitly unavailable: %#v", syncs)
	}
}

// A mount added to the recipe after the first checkout is materialized on
// the next Sync, reported CheckedOut, without disturbing the mounts that
// were already there.
func TestSyncMountsMaterializesAMountAddedAfterCheckout(t *testing.T) {
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
	if !found || added.Outcome != catalog.SyncSkipped {
		t.Fatalf("a new mount without writable-worktree capability must fail closed: %#v", syncs)
	}
	if _, err := os.Stat(filepath.Join(dest, "refs", "semantic")); err != nil {
		t.Fatalf("the new mount must actually land on disk: %v", err)
	}
}

// A mount's Path changing since the last checkout is a recipe shape change,
// not a version advance: Sync must refuse rather than guess how to move it.
func TestSyncMountsRejectsAPathChangeSinceCheckout(t *testing.T) {
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

func readPin(t *testing.T, dest string) (catalog.MountCheckoutPin, error) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dest, catalog.MountCheckoutPinFile))
	if err != nil {
		return catalog.MountCheckoutPin{}, err
	}
	var pin catalog.MountCheckoutPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		return catalog.MountCheckoutPin{}, err
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

package catalog_test

import (
	"path/filepath"
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestCheckoutMountsReportsAuthorityWithoutLocalWorktreeAsSkipped(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/public/semantic")
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{{
		Repository: repo.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("refs/semantic"),
	}}); err != nil {
		t.Fatal(err)
	}
	mounts, err := cat.CheckoutMounts("notes", filepath.Join(testkit.TempDir(t), "work"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || !mounts[0].Skipped || !strings.Contains(mounts[0].Reason, "local git directory") {
		t.Fatalf("authority without a local worktree must be explicit: %#v", mounts)
	}
}

func TestCheckoutMountsRequiresDeclaredPaths(t *testing.T) {
	store := snapshot.NewRegistry()
	repo := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("v", 1, []catalog.WorkspaceSource{{Repository: repo.ID(), Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	_, err := cat.CheckoutMounts("v", filepath.Join(testkit.TempDir(t), "work"))
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

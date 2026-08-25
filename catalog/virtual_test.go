package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// A raw path read routes to the owning mount and returns exact bytes, no
// real checkout ever touching disk.
func TestReadVirtualFileRoutesAndReadsRawBytes(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	core2, ok := storeMemberRawFileStore(t, cat, "kr://acme/public/core2")
	if !ok {
		t.Fatal("fixture must be a RawFileStore")
	}
	root, err := core2.snapshot.Head("refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	commit, err := core2.raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: "kr://acme/public/core2", TargetRef: "refs/heads/main",
		BaseCommit: root, ExpectedTargetCommit: root,
		Changes: []snapshot.TreeChange{{Path: "metrics/dau.md", Content: []byte("daily actives\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = commit

	file, err := cat.ReadVirtualFile("notes", "refs/semantic/metrics/dau.md")
	if err != nil {
		t.Fatal(err)
	}
	if file.Repository != "kr://acme/public/core2" || string(file.Content) != "daily actives\n" {
		t.Fatalf("unexpected virtual read: %#v", file)
	}
}

// A path nobody's mount owns is refused the same way RouteMount refuses it
// directly — ReadVirtualFile does not invent a fallback.
func TestReadVirtualFileRejectsUnownedPath(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := cat.ReadVirtualFile("notes", "analysis/churn.md")
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

// The virtual listing enumerates every mount's files under their composed
// paths, and SubPath reattaches correctly — RouteMount's inverse.
func TestListVirtualFilesCoversAllMountsAndAppliesSubPath(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("notes", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main",
			Path: catalog.MountPath("kb"), SubPath: "docs/knowledge"},
	}); err != nil {
		t.Fatal(err)
	}
	root1, root2 := memberHeads(t, cat, "kr://acme/public/core", "kr://acme/public/core2")

	raw1, _ := storeMemberRawFileStore(t, cat, "kr://acme/public/core")
	if _, err := raw1.raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: root1, ExpectedTargetCommit: root1,
		Changes: []snapshot.TreeChange{{Path: "analysis/churn.md", Content: []byte("x")}},
	}); err != nil {
		t.Fatal(err)
	}
	raw2, _ := storeMemberRawFileStore(t, cat, "kr://acme/public/core2")
	if _, err := raw2.raw.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: "kr://acme/public/core2", TargetRef: "refs/heads/main",
		BaseCommit: root2, ExpectedTargetCommit: root2,
		Changes: []snapshot.TreeChange{
			{Path: "docs/knowledge/metrics/dau.md", Content: []byte("x")},
			{Path: "docs/other/ignored.md", Content: []byte("outside the mounted subtree")},
		},
	}); err != nil {
		t.Fatal(err)
	}

	entries, err := cat.ListVirtualFiles("notes")
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, e := range entries {
		paths[e.Path] = true
	}
	if !paths["analysis/churn.md"] {
		t.Fatalf("root mount's file must be listed verbatim: %v", paths)
	}
	if !paths["kb/metrics/dau.md"] {
		t.Fatalf("subPath must be stripped then the mount path reattached: %v", paths)
	}
	if paths["kb/other/ignored.md"] || paths["docs/other/ignored.md"] {
		t.Fatalf("a file outside the mounted subPath must not surface at all: %v", paths)
	}
}

// A member without RawFileStore is left out of the listing, not an error for
// the whole call — mirrors CheckoutMounts' Skipped handling for capability.
func TestListVirtualFilesSkipsMemberWithoutCapability(t *testing.T) {
	store := snapshot.NewRegistry()
	writable := testkit.MakeRepository(t, "kr://acme/public/core")
	plain := plainSnapshot{Store: testkit.MakeRepository(t, "kr://acme/public/core2")}
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
	entries, err := cat.ListVirtualFiles("notes")
	if err != nil {
		t.Fatalf("a capability gap on one member must not fail the listing: %v", err)
	}
	for _, e := range entries {
		if e.Repository == plain.ID() {
			t.Fatalf("member without RawFileStore must not appear in the listing: %#v", e)
		}
	}
}

type rawMember struct {
	snapshot snapshot.Store
	raw      snapshot.TreeStore
}

func storeMemberRawFileStore(t *testing.T, cat *catalog.Catalog, id string) (rawMember, bool) {
	t.Helper()
	store, err := cat.Require(kernel.RepositoryID(id))
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := snapshot.TreeStoreOf(store)
	return rawMember{snapshot: store, raw: raw}, ok
}

func memberHeads(t *testing.T, cat *catalog.Catalog, ids ...string) (kernel.CommitID, kernel.CommitID) {
	t.Helper()
	heads := make([]kernel.CommitID, len(ids))
	for i, id := range ids {
		snapshot, err := cat.Require(kernel.RepositoryID(id))
		if err != nil {
			t.Fatal(err)
		}
		head, err := snapshot.Head("refs/heads/main")
		if err != nil {
			t.Fatal(err)
		}
		heads[i] = head
	}
	return heads[0], heads[1]
}

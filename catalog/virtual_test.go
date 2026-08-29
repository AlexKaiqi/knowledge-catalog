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

func TestCheckResolvedRepositoryDoesNotProbeOtherMembers(t *testing.T) {
	registry := snapshot.NewRegistry()
	targetChecks, otherChecks := 0, 0
	target := &countingStore{Store: testkit.MakeRepository(t, "kr://acme/target"), checks: &targetChecks}
	other := &countingStore{Store: testkit.MakeRepository(t, "kr://acme/other"), checks: &otherChecks}
	if err := registry.Add(target); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(other); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, registry)
	if _, err := cat.DefineWorkspace("agent", 1, []catalog.WorkspaceSource{
		{Repository: target.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("")},
		{Repository: other.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("other")},
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := cat.ResolveWorkspace("agent")
	if err != nil {
		t.Fatal(err)
	}
	if check := cat.CheckResolvedRepository(resolved, target.ID()); check.Outcome != "PASSED" {
		t.Fatalf("target check = %#v", check)
	}
	if targetChecks != 1 || otherChecks != 0 {
		t.Fatalf("HasCommit calls: target=%d other=%d", targetChecks, otherChecks)
	}
}

type countingStore struct {
	snapshot.Store
	checks *int
}

func (s *countingStore) HasCommit(commit kernel.CommitID) bool {
	(*s.checks)++
	return s.Store.HasCommit(commit)
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

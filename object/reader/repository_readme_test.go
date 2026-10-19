package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// treeOnly hides any native unit locator of the fixture so the Reader sees
// only tree bytes, as with a lakeFS branch.
type treeOnly struct {
	snapshot.Store
	snapshot.TreeStore
}

// README.md has no meaning to the Reader: a file committed without Writer
// locators is not an object, whatever its frontmatter declares.
func TestGitCommittedReadmeWithoutLocatorIsNotAnObject(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://acme/payments")
	tree, ok := snapshot.TreeStoreOf(raw)
	if !ok {
		t.Fatal("tree store")
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	content := "---\nentity: fixture/payments\naspect: readme\nschema_ref: schema/core/readme/v1\n---\n# Payments warehouse\n"
	commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     raw.ID(),
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           root,
		ExpectedTargetCommit: root,
		Changes:              []snapshot.TreeChange{{Path: "README.md", Content: []byte(content)}},
		Message:              "business git commit",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(nil).Wrap(&treeOnly{Store: raw, TreeStore: tree}, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Read("fixture/payments", commit); err == nil {
		t.Fatal("README.md without a locator became an object")
	}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: "readme"}
	if _, err := repo.ReadAddress(address, commit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("README.md without a locator became an exact unit: %v", err)
	}
	ref := objectmodel.Reference{Repository: raw.ID(), Object: address.ObjectID, Locator: "/readme/body"}
	if _, err := repo.(objectmodel.ReferenceReader).ReadReference(ref, commit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("README.md without a locator became a reference target: %v", err)
	}
	page, err := repo.(objectmodel.SnapshotObjectPager).ObjectIDsPage(commit, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.ObjectIDs) != 0 {
		t.Fatalf("object page listed a file without a locator: %#v", page)
	}
}

func TestPlainMarkdownReadmeIsNotObject(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://acme/payments")
	tree, ok := snapshot.TreeStoreOf(raw)
	if !ok {
		t.Fatal("tree store")
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     raw.ID(),
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           root,
		ExpectedTargetCommit: root,
		Changes:              []snapshot.TreeChange{{Path: "README.md", Content: []byte("# Payments\n\nNo frontmatter.\n")}},
		Message:              "plain readme",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Read("fixture/payments", commit); err == nil {
		t.Fatal("plain markdown must not be readable as object")
	}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: "readme"}
	if _, err := repo.ReadAddress(address, commit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("plain markdown became an exact object unit: %v", err)
	}
	resolved, err := repo.ResolveAddress(address, commit)
	if err != nil || resolved.Status != objectmodel.StatusUnresolved {
		t.Fatalf("plain markdown gained a declared Address: %#v %v", resolved, err)
	}
}

package reader_test

import (
	"fmt"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type readmeUnitGuard struct {
	snapshot.Store
	snapshot.TreeStore
	readmeReads int
	blockReadme bool
}

func (g *readmeUnitGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if path == objectmodel.RepositoryReadmePath {
		g.readmeReads++
		if g.blockReadme {
			return nil, fmt.Errorf("unpublished README body was read")
		}
	}
	return g.TreeStore.ReadFile(path, at)
}

func TestReadmeGitCommitWithoutWriterLocators(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://acme/payments")
	tree, ok := snapshot.TreeStoreOf(raw)
	if !ok {
		t.Fatal("tree store")
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	content := "---\nentity: fixture/payments\naspect: readme\nschema_ref: schema/core/readme/v1\n---\n# Payments warehouse\n\nPublished metrics.\n"
	commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     raw.ID(),
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           root,
		ExpectedTargetCommit: root,
		Changes:              []snapshot.TreeChange{{Path: objectmodel.RepositoryReadmePath, Content: []byte(content)}},
		Message:              "business git commit",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("fixture/payments", commit)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := objectmodel.ReadmeBody(value.Value)
	if !ok || body != "# Payments warehouse\n\nPublished metrics." {
		t.Fatalf("read %#v", value.Value)
	}
	changed, err := repo.(objectmodel.FastObjectChanges).FastChangedObjects(root, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].ObjectID != "fixture/payments" || len(changed[0].ToPaths) != 1 {
		t.Fatalf("changed %#v", changed)
	}
	hydrated, err := repo.(objectmodel.UnitPathsHydrator).ReadManyAtPaths(
		[]objectmodel.ObjectID{"fixture/payments"}, commit, map[objectmodel.ObjectID][]string{"fixture/payments": changed[0].ToPaths})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := objectmodel.ReadmeBody(hydrated["fixture/payments"].Value); !ok {
		t.Fatalf("hydrate %#v", hydrated)
	}
	resolved, err := repo.Resolve("fixture/payments", commit)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != objectmodel.StatusResolved || resolved.PathHint != objectmodel.RepositoryReadmePath {
		t.Fatalf("resolve %#v", resolved)
	}
	page, err := repo.(objectmodel.SnapshotObjectPager).ObjectIDsPage(commit, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range page.ObjectIDs {
		if id == "fixture/payments" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("object page must include git-committed README entity: %#v", page)
	}
	guard := &readmeUnitGuard{Store: raw, TreeStore: tree}
	repo, err = reader.NewReader(nil).Wrap(guard, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: objectmodel.ReadmeAspect}
	selected, err := repo.ReadAddress(address, commit)
	if err != nil || selected.Address != address || len(selected.Declarations) != 1 || selected.Declarations[0].SchemaRef != string(objectmodel.CoreReadmeSchemaV1) {
		t.Fatalf("git README lost its exact fixed declaration: %#v %v", selected, err)
	}
	coordinate, err := repo.ResolveAddress(address, commit)
	if err != nil || coordinate.Status != objectmodel.StatusResolved || coordinate.SchemaRef != string(objectmodel.CoreReadmeSchemaV1) || coordinate.PathHint != objectmodel.RepositoryReadmePath {
		t.Fatalf("git README exact resolution did not match its fixed declaration: %#v %v", coordinate, err)
	}
	ref := objectmodel.Reference{Repository: raw.ID(), Object: address.ObjectID, Locator: "/readme/body"}
	leaf, err := repo.(objectmodel.ReferenceReader).ReadReference(ref, commit)
	if err != nil || leaf.Value != body || leaf.Address != address || leaf.Commit != commit {
		t.Fatalf("git README exact reference lost its fixed text: %#v %v", leaf, err)
	}
	paths := repo.(objectmodel.AddressPathsReader)
	if allowed, err := paths.ReadAddressAtPaths(address, commit, []string{objectmodel.RepositoryReadmePath}); err != nil || allowed.Address != address {
		t.Fatalf("published README path was unavailable: %#v %v", allowed, err)
	}
	guard.blockReadme = true
	before := guard.readmeReads
	if _, err := paths.ReadAddressAtPaths(address, commit, []string{"public/allowed.yaml"}); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("unpublished README Address was accepted: %v", err)
	}
	refPaths := repo.(interface {
		ReadReferenceAtPaths(objectmodel.Reference, kernel.CommitID, []string) (objectmodel.ObjectValue, error)
	})
	if _, err := refPaths.ReadReferenceAtPaths(ref, commit, []string{"public/allowed.yaml"}); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("unpublished README reference was accepted: %v", err)
	}
	if guard.readmeReads != before {
		t.Fatal("excluded README was read before its published path was checked")
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
		Changes:              []snapshot.TreeChange{{Path: objectmodel.RepositoryReadmePath, Content: []byte("# Payments\n\nNo frontmatter.\n")}},
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
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/payments", AspectName: objectmodel.ReadmeAspect}
	if _, err := repo.ReadAddress(address, commit); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("plain markdown became an exact object unit: %v", err)
	}
	resolved, err := repo.ResolveAddress(address, commit)
	if err != nil || resolved.Status != objectmodel.StatusUnresolved {
		t.Fatalf("plain markdown gained a declared Address: %#v %v", resolved, err)
	}
}

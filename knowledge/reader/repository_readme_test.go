package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestReadmeGitCommitWithoutWriterLocators(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://acme/payments")
	tree, ok := snapshot.TreeStoreOf(raw)
	if !ok {
		t.Fatal("tree store")
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	content := "---\nentity: payments\naspect: readme\nschema_ref: schema/core/readme/v1\n---\n# Payments warehouse\n\nPublished metrics.\n"
	commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     raw.ID(),
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           root,
		ExpectedTargetCommit: root,
		Changes:              []snapshot.TreeChange{{Path: knowledge.RepositoryReadmePath, Content: []byte(content)}},
		Message:              "business git commit",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	value, err := repo.Read("payments", commit)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := knowledge.ReadmeBody(value.Value)
	if !ok || body != "# Payments warehouse\n\nPublished metrics." {
		t.Fatalf("read %#v", value.Value)
	}
	changed, err := repo.(knowledge.FastObjectChanges).FastChangedObjects(root, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].ObjectID != "payments" || len(changed[0].ToPaths) != 1 {
		t.Fatalf("changed %#v", changed)
	}
	hydrated, err := repo.(knowledge.UnitPathsHydrator).ReadManyAtPaths(
		[]knowledge.ObjectID{"payments"}, commit, map[knowledge.ObjectID][]string{"payments": changed[0].ToPaths})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := knowledge.ReadmeBody(hydrated["payments"].Value); !ok {
		t.Fatalf("hydrate %#v", hydrated)
	}
	resolved, err := repo.Resolve("payments", commit)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != knowledge.StatusResolved || resolved.PathHint != knowledge.RepositoryReadmePath {
		t.Fatalf("resolve %#v", resolved)
	}
	page, err := repo.(knowledge.SnapshotObjectPager).ObjectIDsPage(commit, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range page.ObjectIDs {
		if id == "payments" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("object page must include git-committed README entity: %#v", page)
	}
}

func TestPlainMarkdownReadmeIsNotKnowledge(t *testing.T) {
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
		Changes:              []snapshot.TreeChange{{Path: knowledge.RepositoryReadmePath, Content: []byte("# Payments\n\nNo frontmatter.\n")}},
		Message:              "plain readme",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(nil).Wrap(raw, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Read("payments", commit); err == nil {
		t.Fatal("plain markdown must not be readable as knowledge")
	}
}

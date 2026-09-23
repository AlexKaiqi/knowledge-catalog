package catalog_test

import (
	"fmt"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// Per-file dataset entries (docs/reviewed/dataset.md U10): publish carries
// them as DatasetItemFile items with one frozen commit per repository, and
// the delivered-target rules reject ambiguous layouts at publish instead of
// resolving them by source order.

func writeStoreFiles(t *testing.T, s fed, repoID kernel.RepositoryID, files map[string]string) kernel.CommitID {
	t.Helper()
	snap, ok := s.store.Get(repoID)
	if !ok {
		t.Fatalf("repository %s is not in the fixture store", repoID)
	}
	tree, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		t.Fatalf("repository %s does not support raw tree commits", repoID)
	}
	head, err := snap.Head("refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	changes := make([]snapshot.TreeChange, 0, len(files))
	for path, content := range files {
		changes = append(changes, snapshot.TreeChange{Path: path, Content: []byte(content)})
	}
	commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repoID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head, Changes: changes,
	})
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func fileEntrySources() []catalog.KnowledgeSetSource {
	return []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("reference/policies"), SubPath: "handbook/policies"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", File: "reports/gmv.csv", Target: "reference/policies/gmv.csv"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", File: "reports/notes.txt", Target: "reference/renamed.txt"},
	}
}

func setupFileEntries(t *testing.T) (fed, kernel.CommitID) {
	t.Helper()
	s := setupFed(t)
	publicCommit := writeStoreFiles(t, s, "kr://acme/public/core", map[string]string{
		"handbook/policies/README.md": "policy v1\n",
	})
	groupCommit := writeStoreFiles(t, s, "kr://acme/groups/payments", map[string]string{
		"reports/gmv.csv":   "gmv\n",
		"reports/notes.txt": "notes\n",
	})
	_ = groupCommit
	return s, publicCommit
}

func TestPublishedDatasetCarriesFileEntriesAsItems(t *testing.T) {
	s, publicCommit := setupFileEntries(t)
	defined, err := s.catalog.DefineKnowledgeSet("mixed", 1, fileEntrySources())
	if err != nil {
		t.Fatal(err)
	}
	if len(defined.Items) != 3 {
		t.Fatalf("publish must store one item per entry: %#v", defined.Items)
	}
	fileItems := catalog.DatasetFileItems(defined.Items)
	if len(fileItems) != 2 {
		t.Fatalf("two per-file entries expected: %#v", fileItems)
	}
	for _, item := range fileItems {
		if item.Kind != catalog.DatasetItemFile || item.Repository != "kr://acme/groups/payments" || item.Commit == "" {
			t.Fatalf("file item must name its source repository and frozen commit: %#v", item)
		}
	}
	if fileItems[0].Target != "reference/policies/gmv.csv" || fileItems[0].File != "reports/gmv.csv" {
		t.Fatalf("file item lost its rename mapping: %#v", fileItems[0])
	}
	if fileItems[1].Target != "reference/renamed.txt" || fileItems[1].File != "reports/notes.txt" {
		t.Fatalf("file item lost its delivered location: %#v", fileItems[1])
	}
	resolved, err := s.catalog.ResolveKnowledgeSet("mixed")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 3 || resolved.Repositories["kr://acme/public/core"] != publicCommit {
		t.Fatalf("resolve must carry the same item list and frozen commits: %#v", resolved)
	}
	mounts, err := catalog.ListVirtualMountsAt(defined, resolved)
	if err != nil || len(mounts) != 1 || mounts[0].Path != "reference/policies" {
		t.Fatalf("per-file entries must not become mounts: mounts=%#v err=%v", mounts, err)
	}
}

func TestDatasetFileEntryTargetConflictsAreRejectedAtPublish(t *testing.T) {
	s := setupFed(t)
	writeStoreFiles(t, s, "kr://acme/public/core", map[string]string{"handbook/policies/README.md": "policy v1\n"})
	writeStoreFiles(t, s, "kr://acme/groups/payments", map[string]string{"reports/gmv.csv": "gmv\n"})
	for i, tc := range []struct {
		name    string
		sources []catalog.KnowledgeSetSource
	}{
		{
			name: "duplicate file target",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", File: "a.txt", Target: "same.txt"},
				{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", File: "b.txt", Target: "same.txt"},
			},
		},
		{
			name: "file target takes a delivered directory",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("reference/policies"), SubPath: "handbook/policies"},
				{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", File: "reports/gmv.csv", Target: "reference/policies"},
			},
		},
		{
			name: "delivered directory nests under a file target",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("reference/policies/metrics"), SubPath: "handbook/policies"},
				{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", File: "reports/gmv.csv", Target: "reference/policies"},
			},
		},
		{
			name: "file target is the ancestor of another file target",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", File: "a.txt", Target: "data"},
				{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", File: "b.txt", Target: "data/gmv.csv"},
			},
		},
		{
			name: "traversal in delivered target",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", File: "a.txt", Target: "../escape.txt"},
			},
		},
		{
			name: "traversal in source file",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", File: "../../etc/passwd", Target: "out.txt"},
			},
		},
		{
			name: "file entry declares a mount path",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("docs"), File: "a.txt", Target: "out.txt"},
			},
		},
		{
			name: "mount entry declares a delivered target",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("docs"), Target: "out.txt"},
			},
		},
		{
			name: "file entry splits repository coordinates",
			sources: []catalog.KnowledgeSetSource{
				{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("docs"), SubPath: "handbook"},
				{Repository: "kr://acme/public/core", Selector: "refs/heads/other", File: "a.txt", Target: "out.txt"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A unique dataset per case: one case's rejected define must not
			// poison the next case with a stale v1 (NON_FAST_FORWARD noise).
			name := fmt.Sprintf("conflict-%d", i)
			_, err := s.catalog.DefineKnowledgeSet(name, 1, tc.sources)
			testkit.ExpectCode(t, err, kernel.ErrKnowledgeSetInvalid)
			if _, getErr := s.catalog.Set(name); getErr == nil {
				t.Fatalf("rejected candidate must not define the dataset: %v", getErr)
			}
		})
	}
}

func TestDatasetRecipeRoundTripsFileEntries(t *testing.T) {
	s, _ := setupFileEntries(t)
	defined, err := s.catalog.DefineKnowledgeSet("portable", 1, fileEntrySources())
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := catalog.RecipeFromKnowledgeSet(defined)
	if !ok || len(rec.Files) != 2 || len(rec.Mounts) != 1 {
		t.Fatalf("recipe must carry mounts and file entries: %#v ok=%v", rec, ok)
	}
	raw, err := catalog.FormatKnowledgeSetRecipe(rec)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := catalog.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		t.Fatal(err)
	}
	sources := parsed.Sources()
	if len(sources) != len(defined.Sources) {
		t.Fatalf("recipe sources lost entries: %#v", sources)
	}
	for i, src := range sources {
		want := defined.Sources[i]
		// The recipe is the portable shape: it deliberately does not carry the
		// frozen commit — the next publish refreezes it.
		want.Commit, want.BaseRev = "", ""
		samePath := (src.Path == nil) == (want.Path == nil) &&
			(src.Path == nil || *src.Path == *want.Path)
		if !samePath || src.Repository != want.Repository || src.Selector != want.Selector ||
			src.File != want.File || src.Target != want.Target || src.SubPath != want.SubPath {
			t.Fatalf("recipe source %d diverged: got %#v want %#v", i, src, want)
		}
	}
}

func TestDatasetOverlayMergesFileEntries(t *testing.T) {
	s, _ := setupFileEntries(t)
	defined, err := s.catalog.DefineKnowledgeSet("overlay-files", 1, fileEntrySources())
	if err != nil {
		t.Fatal(err)
	}
	over, err := catalog.ParseKnowledgeSetOverlay([]byte(`name: overlay-files
files:
  - repository: kr://acme/groups/payments
    selector: refs/heads/main
    file: reports/gmv.csv
    target: reference/renamed.txt
`))
	if err != nil {
		t.Fatal(err)
	}
	merged, err := catalog.MergeOverlay(defined, over)
	if err != nil {
		t.Fatal(err)
	}
	if items := catalog.DatasetFileItems(merged.Items); len(items) != 0 {
		t.Fatalf("merged view is unpublished; items must be re-derived: %#v", items)
	}
	resolved, err := s.catalog.ResolveDefinition(merged)
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]string{}
	for _, item := range catalog.DatasetFileItems(resolved.Items) {
		targets[item.Target] = item.File
	}
	if len(targets) != 2 || targets["reference/renamed.txt"] != "reports/gmv.csv" {
		t.Fatalf("overlay file must replace by delivered target: %#v", targets)
	}
	// The entry that previously held reference/renamed.txt (notes.txt) is gone;
	// the same source file may still be delivered at its other target.
	for _, file := range targets {
		if file == "reports/notes.txt" {
			t.Fatalf("the replaced file entry's old source must be gone: %#v", targets)
		}
	}
}

func TestDatasetFileEntriesChangeThePin(t *testing.T) {
	mounts := []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("docs"), SubPath: "handbook"},
	}
	commits := map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/core": "c1"}
	base := catalog.HashResolved("pinned", mounts, commits)
	withFile := catalog.HashResolved("pinned", append(mounts, catalog.KnowledgeSetSource{
		Repository: "kr://acme/public/core", Selector: "refs/heads/main", File: "a.txt", Target: "out.txt",
	}), commits)
	if base == withFile {
		t.Fatal("a per-file entry changes what a consumer reads; it must change the pin")
	}
	same := catalog.HashResolved("pinned", []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("docs"), SubPath: "handbook"},
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", File: "a.txt", Target: "out.txt"},
	}, commits)
	if withFile != same {
		t.Fatal("pin identity must stay stable for the same file list regardless of entry order")
	}
}

func TestDatasetDeliveredTreeAnswersFileQuestions(t *testing.T) {
	items := []catalog.DatasetItem{
		{Kind: catalog.DatasetItemFile, Repository: "kr://acme/public/core", Target: "data/metrics/gmv.csv", File: "reports/gmv.csv"},
		{Kind: catalog.DatasetItemFile, Repository: "kr://acme/groups/payments", Target: "notes.md", File: "docs/notes.md"},
	}
	tree := catalog.NewDatasetDeliveredTree(items)
	if item, ok := tree.Find("data/metrics/gmv.csv"); !ok || item.File != "reports/gmv.csv" {
		t.Fatalf("find must locate the delivered file: %#v ok=%v", item, ok)
	}
	if _, ok := tree.Find("data/metrics"); ok {
		t.Fatal("a delivered directory is not a file entry")
	}
	root := tree.FilesIn("")
	if len(root) != 1 || root[0].Target != "notes.md" {
		t.Fatalf("root listing must name root files: %#v", root)
	}
	if dirs := tree.DirsIn(""); len(dirs) != 1 || dirs[0] != "data" {
		t.Fatalf("file entries must derive their directories: %v", dirs)
	}
	if files := tree.FilesIn("data"); len(files) != 0 {
		t.Fatalf("data holds no direct file: %#v", files)
	}
	if files := tree.FilesIn("data/metrics"); len(files) != 1 || files[0].Target != "data/metrics/gmv.csv" {
		t.Fatalf("nested file entry must be listed in its directory: %#v", files)
	}
}

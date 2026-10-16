package snapshot_test

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestLakeFSEmptyTreeProbeRejectsMovableRefAliases(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://snapshot/empty-probe")
	commit, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	if empty, known, err := snapshot.ProbeEmptyTree(repo, commit); err != nil || !known || !empty {
		t.Fatalf("actual fixed empty lakeFS tree was not recognized: empty=%t known=%t err=%v", empty, known, err)
	}
	for _, alias := range []kernel.CommitID{"main", "refs/heads/main", "HEAD"} {
		t.Run(string(alias), func(t *testing.T) {
			empty, _, err := snapshot.ProbeEmptyTree(repo, alias)
			if empty || kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
				t.Fatalf("movable ref alias became an immutable empty proof: %s empty=%t err=%v", alias, empty, err)
			}
		})
	}
}

func TestLakeFSEmptyTreeProbeCountsUnindexedAndHiddenContent(t *testing.T) {
	for _, path := range []string{"raw.json", ".kc/unknown.json"} {
		t.Run(path, func(t *testing.T) {
			fake := newFakeLakeFS(t)
			repo := fake.open(t, "kr://snapshot/nonempty-probe")
			base, err := repo.Head(snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			commit, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
				TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
				BaseCommit: base, ExpectedTargetCommit: base,
				Changes: []snapshot.TreeChange{{Path: path, Content: []byte("unindexed content")}},
			})
			if err != nil {
				t.Fatal(err)
			}
			empty, known, err := snapshot.ProbeEmptyTree(repo, commit)
			if err != nil || !known || empty {
				t.Fatalf("literal lakeFS content became an empty proof: empty=%t known=%t err=%v", empty, known, err)
			}
		})
	}
}

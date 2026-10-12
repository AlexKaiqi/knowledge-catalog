package snapshot_test

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestRepositoryDeclaresReplaceableSnapshotCapabilities(t *testing.T) {
	repo := newFakeLakeFS(t).open(t, "kr://conformance/lakefs-capabilities")
	for name, present := range map[string]bool{
		"store":     hasCapability[snapshot.Store](repo),
		"origin":    hasCapability[snapshot.StoreOrigin](repo),
		"tree":      hasCapability[snapshot.TreeStore](repo),
		"directory": hasCapability[snapshot.DirectoryReader](repo),
		"history":   hasCapability[snapshot.HistoryStore](repo),
		"changes":   hasCapability[snapshot.ChangeStore](repo),
		"bulk":      hasCapability[snapshot.BulkTreeIngester](repo),
		"metadata":  hasCapability[snapshot.CommitMetadataReader](repo),
	} {
		if !present {
			t.Fatalf("lakeFS factory is missing %s capability", name)
		}
	}
}

func hasCapability[T any](value any) bool {
	_, ok := value.(T)
	return ok
}

func TestLakeFSCommitMetadataPreservesAuthorAcrossReopen(t *testing.T) {
	fake := newFakeLakeFS(t)
	id := kernel.RepositoryID("kr://conformance/lakefs-provenance")
	repo := fake.open(t, string(id))
	base, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: id, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Changes: []snapshot.TreeChange{{Path: "README.md", Content: []byte("literal bytes")}},
		Author:  "alice", Message: "published files", RequestID: "publish-17", RuleID: "files-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Reopen through the public factory so provenance cannot come from the
	// original handle's memory. The native committer remains the service user.
	reopened, err := snapshot.OpenLakeFS(id, fake.server.URL+"/knowledge", "access:secret")
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := reopened.(snapshot.CommitMetadataReader)
	if !ok {
		t.Fatal("lakeFS factory omitted commit metadata capability")
	}
	metadata, err := reader.ReadCommitMetadata(commit)
	if err != nil {
		t.Fatal(err)
	}
	want := snapshot.CommitMetadata{
		Commit: commit, Author: "alice", Message: "published files", RequestID: "publish-17", RuleID: "files-v1",
	}
	if metadata != want {
		t.Fatalf("commit provenance=%#v, want %#v", metadata, want)
	}
	legacy, err := reader.ReadCommitMetadata(base)
	if err != nil || legacy.Commit != base || legacy.Author != "kc" || legacy.RequestID != "" || legacy.RuleID != "" {
		t.Fatalf("native committer fallback=%#v err=%v", legacy, err)
	}
	if _, err := reader.ReadCommitMetadata("missing"); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("missing commit: %v", err)
	}
}

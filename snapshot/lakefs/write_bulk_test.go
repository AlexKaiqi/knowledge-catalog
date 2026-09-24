package lakefs_test

import (
	"bytes"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

// LAKEFS-04 bulk-ingest contract. The bulk apply capability must route object
// bytes through the lakeFS object-upload API (one round trip per object,
// bytes through the lakeFS server) instead of the three-round-trip presigned
// staging dance, while wip/commit/publish/CAS semantics stay identical.

func TestLakeFSBulkTreeIngesterCapability(t *testing.T) {
	repo := newFakeLakeFS(t).open(t, "kr://conformance/lakefs-bulk-capability")
	if _, ok := snapshot.BulkTreeIngesterOf(repo); !ok {
		t.Fatal("lakeFS repository does not advertise the bulk tree ingest capability")
	}
}

func TestLakeFSBulkApplyUsesObjectUploadDataPlane(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/lakefs-bulk-data-plane")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	changes := []snapshot.TreeChange{
		{Path: "objects/bulk-a.bin", Content: []byte("a")},
		{Path: "objects/bulk-b.bin", Content: []byte("b")},
		{Path: "objects/bulk-c.bin", Content: []byte("c")},
	}
	commit, err := repo.ApplyTreeCommitBulk(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, RequestID: "bulk-apply",
		Changes: changes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.uploadWrites != len(changes) {
		t.Fatalf("bulk apply used %d object-upload calls, want %d", fake.uploadWrites, len(changes))
	}
	if fake.stagingGets != 0 || fake.stagingLinks != 0 {
		t.Fatalf("bulk apply reused the staging dance: presign=%d link=%d", fake.stagingGets, fake.stagingLinks)
	}
	if fake.directWrites != 0 {
		t.Fatalf("bulk apply must not bypass the lakeFS data plane: direct writes=%d", fake.directWrites)
	}
	for _, change := range changes {
		got, err := repo.ReadFile(change.Path, commit)
		if err != nil || !bytes.Equal(got, change.Content) {
			t.Fatalf("bulk commit read %s = %q err=%v", change.Path, got, err)
		}
	}
}

func TestLakeFSBulkApplyMatchesStagingSemantics(t *testing.T) {
	// Applied through the bulk data plane, PUTs land, REMOVEs delete, and
	// stale expected-old CAS is still rejected: the resulting tree must be
	// indistinguishable from the staging path.
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/lakefs-bulk-parity")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	seed := snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, RequestID: "parity-seed",
		Changes: []snapshot.TreeChange{
			{Path: "objects/keep.bin", Content: []byte("keep")},
			{Path: "objects/drop.bin", Content: []byte("drop")},
		},
	}
	if _, err := repo.ApplyTreeCommitBulk(seed); err != nil {
		t.Fatal(err)
	}
	base, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.ApplyTreeCommitBulk(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, RequestID: "parity-bulk",
		Changes: []snapshot.TreeChange{
			{Path: "objects/drop.bin", Remove: true},
			{Path: "objects/add.bin", Content: []byte("add")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ReadFile("objects/keep.bin", commit); err != nil || string(got) != "keep" {
		t.Fatalf("keep.bin=%q err=%v", got, err)
	}
	if _, err := repo.ReadFile("objects/drop.bin", commit); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("drop.bin should be gone, err=%v", kernel.CodeOf(err))
	}
	if got, err := repo.ReadFile("objects/add.bin", commit); err != nil || string(got) != "add" {
		t.Fatalf("add.bin=%q err=%v", got, err)
	}
	stale := snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, RequestID: "parity-stale",
		Changes: []snapshot.TreeChange{{Path: "objects/late.bin", Content: []byte("late")}},
	}
	if _, err := repo.ApplyTreeCommitBulk(stale); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("stale bulk apply returned %v, want %v", err, kernel.ErrNonFastForward)
	}
}

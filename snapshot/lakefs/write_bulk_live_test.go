package lakefs_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	lakefs "kc/snapshot/lakefs"
)

// TestLiveBulkApplyAgainstRealLakeFS runs the bulk data-plane contract
// against a real lakeFS server. The fake-medium tests prove semantics; this
// test proves the production REST shape: the one-round-trip object upload
// must land bytes on the backing store through the lakeFS server, REMOVEs
// must delete, and stale expected-old CAS must be rejected — with wip,
// commit and publication identical to the presigned path.
func TestLiveBulkApplyAgainstRealLakeFS(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"))
	if origin == "" {
		t.Skip("set KC_SCENE_LIVE_LAKEFS_URL to verify the bulk data plane against real lakeFS")
	}
	cred := strings.TrimSpace(os.Getenv("KC_LAKEFS_CREDENTIAL"))
	if cred == "" {
		t.Fatal("KC_LAKEFS_CREDENTIAL is required for the live bulk verification")
	}
	live := testkit.NewLakeFSLive(t, origin, cred, strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE")))
	repo, err := lakefs.OpenExisting(kernel.RepositoryID("kr://live/bulk-verify"), live.DSN(live.NewRepo()), cred)
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}

	first, err := repo.ApplyTreeCommitBulk(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, RequestID: "live-bulk-put",
		Changes: []snapshot.TreeChange{
			{Path: "objects/keep.bin", Content: []byte("keep")},
			{Path: "objects/drop.bin", Content: []byte("drop")},
		},
	})
	if err != nil {
		t.Fatalf("live bulk put: %v", err)
	}
	if got, err := repo.ReadFile("objects/keep.bin", first); err != nil || !bytes.Equal(got, []byte("keep")) {
		t.Fatalf("live bulk read keep.bin = %q err=%v", got, err)
	}

	base, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ApplyTreeCommitBulk(snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, RequestID: "live-bulk-mixed",
		Changes: []snapshot.TreeChange{
			{Path: "objects/drop.bin", Remove: true},
			{Path: "objects/add.bin", Content: []byte("add")},
		},
	})
	if err != nil {
		t.Fatalf("live bulk mixed: %v", err)
	}
	if got, err := repo.ReadFile("objects/keep.bin", second); err != nil || string(got) != "keep" {
		t.Fatalf("keep.bin = %q err=%v", got, err)
	}
	if _, err := repo.ReadFile("objects/drop.bin", second); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("drop.bin should be gone, err=%v", kernel.CodeOf(err))
	}
	if got, err := repo.ReadFile("objects/add.bin", second); err != nil || string(got) != "add" {
		t.Fatalf("add.bin = %q err=%v", got, err)
	}

	stale := snapshot.TreeChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, RequestID: "live-bulk-stale",
		Changes: []snapshot.TreeChange{{Path: "objects/late.bin", Content: []byte("late")}},
	}
	if _, err := repo.ApplyTreeCommitBulk(stale); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("stale live bulk apply returned %v, want %v", err, kernel.ErrNonFastForward)
	}
}

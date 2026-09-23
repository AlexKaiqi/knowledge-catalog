package lakefs_test

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"kc/kernel"
	"kc/snapshot"
)

// TestLakeFSApplyStagesObjectsConcurrently pins the bounded-concurrency write
// path: object staging inside one ApplyTreeCommit must overlap on the private
// wip branch instead of paying one presign+PUT+link round trip per object
// serially. The fake widens each presigned PUT with a sleep and records the
// maximum number of overlapping uploads.
func TestLakeFSApplyStagesObjectsConcurrently(t *testing.T) {
	fake := newFakeLakeFS(t)
	fake.blobSleep = 20 * time.Millisecond
	repo := fake.open(t, "kr://conformance/lakefs-concurrent-apply")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	changes := make([]snapshot.TreeChange, 0, 8)
	for i := range 8 {
		changes = append(changes, snapshot.TreeChange{
			Path:    "objects/" + strconv.Itoa(i) + ".bin",
			Content: []byte(strconv.Itoa(i)),
		})
	}
	if _, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     repo.ID(),
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           root,
		ExpectedTargetCommit: root,
		RequestID:            "concurrent-staging",
		Changes:              changes,
	}); err != nil {
		t.Fatalf("concurrent apply failed: %v", err)
	}
	if got := atomic.LoadInt32(&fake.blobMaxInFlight); got < 2 {
		t.Fatalf("object staging stayed serial: max in-flight presigned PUTs = %d", got)
	}
}

// TestLakeFSRepeatedReadsReuseCommitExistence pins the positive commit
// existence cache: reads at one immutable commit must not re-probe
// GET /commits/{id} per ReadFile. Negatives are never cached, so a commit
// created concurrently stays visible (contract tests cover that path).
func TestLakeFSRepeatedReadsReuseCommitExistence(t *testing.T) {
	fake := newFakeLakeFS(t)
	repo := fake.open(t, "kr://conformance/lakefs-commit-memo")
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := repo.ReadFile("objects/missing.bin", root); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
			t.Fatalf("read %d: want missing-object error, got %v", i, err)
		}
	}
	if fake.commitLookups != 1 {
		t.Fatalf("commit existence was probed %d times for 3 reads at one commit", fake.commitLookups)
	}
}

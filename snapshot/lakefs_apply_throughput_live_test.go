package snapshot_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// TestLiveApplyThroughput measures one many-small-object commit against a real
// lakeFS for every transport the adapter offers. It is an opt-in measurement
// harness (KC_LAKEFS_BENCH_OBJECTS), not a timing assertion: it reports
// wall time and objects per second and only fails on a broken apply.
func TestLiveApplyThroughput(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"))
	objects, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("KC_LAKEFS_BENCH_OBJECTS")))
	if origin == "" || objects <= 0 {
		t.Skip("set KC_SCENE_LIVE_LAKEFS_URL and KC_LAKEFS_BENCH_OBJECTS to measure live apply throughput")
	}
	cred := strings.TrimSpace(os.Getenv("KC_LAKEFS_CREDENTIAL"))
	if cred == "" {
		t.Fatal("KC_LAKEFS_CREDENTIAL is required for the live throughput measurement")
	}
	size, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("KC_LAKEFS_BENCH_BYTES")))
	if size <= 0 {
		size = 512
	}
	live := testkit.NewLakeFSLive(t, origin, cred, strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE")))
	modes := strings.Split(envOr("KC_LAKEFS_BENCH_MODES", "apply,bulk"), ",")
	for _, mode := range modes {
		mode = strings.TrimSpace(mode)
		t.Run(mode, func(t *testing.T) {
			repo, err := openLakeFSForTest(kernel.RepositoryID("kr://live/bench-"+mode), live.DSN(live.NewRepo()), cred)
			if err != nil {
				t.Fatal(err)
			}
			root, err := repo.Head(snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			cs := benchChangeSet(repo.ID(), root, objects, size, mode)
			apply := map[string]func(snapshot.TreeChangeSet) (kernel.CommitID, error){
				"apply": repo.ApplyTreeCommit,
				"bulk":  repo.ApplyTreeCommitBulk,
			}[mode]
			if apply == nil {
				t.Fatalf("unknown bench mode %q", mode)
			}
			started := time.Now()
			commit, err := apply(cs)
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("%s apply: %v", mode, err)
			}
			raw, err := repo.ReadFile(cs.Changes[len(cs.Changes)-1].Path, commit)
			if err != nil || len(raw) != size {
				t.Fatalf("%s apply lost the last object: len=%d err=%v", mode, len(raw), err)
			}
			t.Logf("mode=%s objects=%d bytes=%d elapsed=%s objects/s=%.0f", mode, objects, size,
				elapsed.Round(time.Millisecond), float64(objects)/elapsed.Seconds())
		})
	}
}

func benchChangeSet(id kernel.RepositoryID, root kernel.CommitID, objects, size int, label string) snapshot.TreeChangeSet {
	changes := make([]snapshot.TreeChange, 0, objects)
	for index := 0; index < objects; index++ {
		content := []byte(strings.Repeat(fmt.Sprintf("%s-%08d|", label, index), size/10+1))[:size]
		changes = append(changes, snapshot.TreeChange{Path: fmt.Sprintf("objects/%03d/%06d.json", index%256, index), Content: content})
	}
	return snapshot.TreeChangeSet{
		TargetRepository: id, TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root, RequestID: "live-bench-" + label, Changes: changes,
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

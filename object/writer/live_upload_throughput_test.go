package writer_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

// TestLiveWriterUploadThroughput measures end-to-end Writer commits against a
// real lakeFS: an initial upload into an empty repository and a full update of
// the same objects on the now non-empty basis. It is an opt-in measurement
// (KC_LAKEFS_BENCH_OBJECTS) that logs wall time per phase and fails only when
// a commit is broken. Run it next to lakeFS so presigned URLs resolve.
func TestLiveWriterUploadThroughput(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"))
	objects, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("KC_LAKEFS_BENCH_OBJECTS")))
	if origin == "" || objects <= 0 {
		t.Skip("set KC_SCENE_LIVE_LAKEFS_URL and KC_LAKEFS_BENCH_OBJECTS to measure live Writer throughput")
	}
	cred := strings.TrimSpace(os.Getenv("KC_LAKEFS_CREDENTIAL"))
	if cred == "" {
		t.Fatal("KC_LAKEFS_CREDENTIAL is required for the live Writer measurement")
	}
	bulk := strings.TrimSpace(os.Getenv("KC_LAKEFS_BENCH_BULK")) == "1"
	live := testkit.NewLakeFSLive(t, origin, cred, strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE")))
	store, err := snapshot.OpenLakeFS(kernel.RepositoryID("kr://live/writer-bench"), live.DSN(live.NewRepo()), cred)
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(store); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, store, snapshot.DefaultRef)
	for _, phase := range []string{"initial", "update"} {
		cs := liveWriterChanges(store.ID(), head, objects, phase)
		cs.BulkIngest = bulk
		started := time.Now()
		receipt, err := w.Commit("live-writer-"+phase, cs)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("%s commit: %v", phase, err)
		}
		head = receipt.Result.NewCommit
		t.Logf("phase=%s bulk=%v objects=%d elapsed=%s objects/s=%.0f", phase, bulk, objects,
			elapsed.Round(time.Millisecond), float64(objects)/elapsed.Seconds())
	}
}

func liveWriterChanges(id kernel.RepositoryID, base kernel.CommitID, objects int, label string) objectmodel.ChangeSet {
	operations := []objectmodel.Operation{{
		Op:      objectmodel.OpPut,
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/bench-note"},
		Value: map[string]any{"entity": "BenchNote", "fields": map[string]any{
			"label": map[string]any{"type": "string", "required": true},
			"body":  map[string]any{"type": "string"},
		}},
	}}
	for index := 0; index < objects; index++ {
		operations = append(operations, objectmodel.Operation{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(fmt.Sprintf("note/%06d", index))},
			SchemaRef: "schema/bench-note", PathHint: fmt.Sprintf("notes/%03d/%06d.json", index%256, index),
			Value: map[string]any{
				"label": fmt.Sprintf("%s %d", label, index),
				"body":  strings.Repeat(label+" body ", 32),
			},
		})
	}
	return objectmodel.ChangeSet{
		TargetRepository: id, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Operations: operations,
	}
}

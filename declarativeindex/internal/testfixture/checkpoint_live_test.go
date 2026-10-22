package testfixture

import (
	"os"
	"strings"
	"testing"
)

// TestLiveSnapshotCheckpointOverwritesPollutedTarget covers the engine behaviour
// the fixture has to survive: a same-name restore is refused outright. The fixture
// must not rely on the engine refusing, so it deletes first and then restores.
func TestLiveSnapshotCheckpointOverwritesPollutedTarget(t *testing.T) {
	endpoint := os.Getenv("KC_VERIFY_OPENSEARCH_URL")
	repository := os.Getenv("KC_VERIFY_SNAPSHOT_REPO")
	if endpoint == "" || repository == "" {
		t.Skip("live verification requires KC_VERIFY_OPENSEARCH_URL and KC_VERIFY_SNAPSHOT_REPO")
	}
	checkpoint, err := NewSnapshotCheckpoint(endpoint, repository)
	if err != nil {
		t.Fatal(err)
	}
	source, reuse := "live-plain", "live-plain-reuse"
	prefix := ProjectionPrefix(source)
	target := ProjectionPrefix(reuse)
	for _, owned := range []string{prefix, target} {
		if err := checkpoint.dropOwned(owned); err != nil {
			t.Fatalf("pre-clean %s: %v", owned, err)
		}
	}
	t.Cleanup(func() {
		_ = checkpoint.dropOwned(prefix)
		_ = checkpoint.dropOwned(target)
	})
	const basis = "b1c2d3e4f5061728394a5b6c7d8e9f01b2c3d4e5f60718293a4b5c6d7e8f901"
	if err := checkpoint.do("PUT", "/"+prefix+"objects", map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": map[string]any{"properties": map[string]any{
			"source_repository": map[string]any{"type": "keyword"},
			"source_commit":     map[string]any{"type": "keyword"},
		}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := checkpoint.do("POST", "/"+prefix+"objects/_doc?refresh=true", map[string]any{
			"id": id, "source_repository": "scnllivecap01", "source_commit": basis,
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	captured, err := checkpoint.Capture("kc-cp-plain", "fp-plain", source)
	if err != nil {
		t.Fatal(err)
	}
	// The engine refuses both shapes: same-name and rename-onto-existing. The
	// fixture must not rely on either error, so it deletes first.
	if err := checkpoint.do("POST", "/"+prefix+"objects/_doc?refresh=true", map[string]any{"dirty": true}, nil); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"indices": prefix + "objects", "include_global_state": false}
	err = checkpoint.do("POST", "/_snapshot/"+repository+"/"+captured.Snapshot+"/_restore?wait_for_completion=true", body, nil)
	if err == nil {
		t.Fatalf("same-name restore unexpectedly succeeded; the engine now overwrites, so revisit the guard")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("same-name restore failed for an unexpected reason: %v", err)
	}

	if err := checkpoint.do("PUT", "/"+target+"objects", map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Restore(captured, "fp-plain", captured.Sources, reuse); err != nil {
		t.Fatalf("restore over a polluted target: %v", err)
	}
	restored, _, err := checkpoint.ownedCounts(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored[target+"objects"]; got != 2 {
		t.Fatalf("restored documents = %d, want 2", got)
	}
}

// TestLiveSnapshotCheckpoint exercises the checkpoint against a real cluster.
// Set KC_VERIFY_OPENSEARCH_URL and KC_VERIFY_SNAPSHOT_REPO to run it; the cluster
// must have path.repo configured and the repository registered.
func TestLiveSnapshotCheckpoint(t *testing.T) {
	endpoint := os.Getenv("KC_VERIFY_OPENSEARCH_URL")
	repository := os.Getenv("KC_VERIFY_SNAPSHOT_REPO")
	if endpoint == "" || repository == "" {
		t.Skip("live verification requires KC_VERIFY_OPENSEARCH_URL and KC_VERIFY_SNAPSHOT_REPO")
	}
	checkpoint, err := NewSnapshotCheckpoint(endpoint, repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.do("PUT", "/_snapshot/"+repository, map[string]any{
		"type": "fs", "settings": map[string]any{"location": "/mnt/snapshots"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	source, reuse := "live-capture", "live-reuse"
	prefix := ProjectionPrefix(source)
	targetPrefix := ProjectionPrefix(reuse)
	// Make the test repeatable: a previous interrupted run leaves owned indices
	// behind, and creating them again fails before any assertion runs.
	if err := checkpoint.dropOwned(prefix); err != nil {
		t.Fatalf("pre-clean source: %v", err)
	}
	if err := checkpoint.dropOwned(targetPrefix); err != nil {
		t.Fatalf("pre-clean target: %v", err)
	}
	t.Cleanup(func() {
		_ = checkpoint.dropOwned(prefix)
		_ = checkpoint.dropOwned(targetPrefix)
	})
	if err := checkpoint.do("PUT", "/"+prefix+"objects", map[string]any{
		"settings": map[string]any{"number_of_shards": 2, "number_of_replicas": 0},
		"mappings": map[string]any{"properties": map[string]any{
			"source_repository": map[string]any{"type": "keyword"},
			"source_commit":     map[string]any{"type": "keyword"},
		}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	const basis = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	for _, id := range []string{"a", "b", "c"} {
		if err := checkpoint.do("POST", "/"+prefix+"objects/_doc?refresh=true", map[string]any{
			"id": id, "source_repository": "scnllivecap01", "source_commit": basis,
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	captured, err := checkpoint.Capture("kc-cp-live", "fp-live", source)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("captured documents=%v shards=%d", captured.Documents, captured.Shards)

	target := ProjectionPrefix(reuse)
	if err := checkpoint.do("PUT", "/"+target+"objects", map[string]any{
		"settings": map[string]any{"number_of_shards": 2, "number_of_replicas": 0},
	}, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := checkpoint.do("POST", "/"+target+"objects/_doc?refresh=true", map[string]any{"stale": i}, nil); err != nil {
			t.Fatal(err)
		}
	}
	before, _, err := checkpoint.ownedCounts(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("polluted target before restore=%v", before)

	if err := checkpoint.Restore(captured, "fp-live", captured.Sources, reuse); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after, _, err := checkpoint.ownedCounts(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("target after restore=%v", after)
	if got := after[target+"objects"]; got != 3 {
		t.Fatalf("restored documents = %d, want 3 (stale documents survived)", got)
	}
}

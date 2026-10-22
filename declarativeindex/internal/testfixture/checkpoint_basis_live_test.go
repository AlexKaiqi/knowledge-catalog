package testfixture

import (
	"os"
	"strings"
	"testing"
)

// TestLiveCapturedSourcesAreReadFromRealDocuments confirms the source identity
// aggregation against a real cluster. The fake cluster replies with the shape
// directly, so only a live run proves multi_terms reaches the stored fields.
func TestLiveCapturedSourcesAreReadFromRealDocuments(t *testing.T) {
	endpoint := os.Getenv("KC_VERIFY_OPENSEARCH_URL")
	repository := os.Getenv("KC_VERIFY_SNAPSHOT_REPO")
	if endpoint == "" || repository == "" {
		t.Skip("live verification requires KC_VERIFY_OPENSEARCH_URL and KC_VERIFY_SNAPSHOT_REPO")
	}
	checkpoint, err := NewSnapshotCheckpoint(endpoint, repository)
	if err != nil {
		t.Fatal(err)
	}
	namespace := "live-sources"
	prefix := ProjectionPrefix(namespace)
	if err := checkpoint.dropOwned(prefix); err != nil {
		t.Fatalf("pre-clean: %v", err)
	}
	t.Cleanup(func() { _ = checkpoint.dropOwned(prefix) })

	if err := checkpoint.do("PUT", "/"+prefix+"objects", map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": map[string]any{"properties": map[string]any{
			"object_id":         map[string]any{"type": "keyword"},
			"source_repository": map[string]any{"type": "keyword"},
			"source_commit":     map[string]any{"type": "keyword"},
		}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	// Two repositories at two commits: the identity is a pair, not a single commit.
	docs := []map[string]any{
		{"object_id": "a", "source_repository": "scnlabc123", "source_commit": "commit-one"},
		{"object_id": "b", "source_repository": "scnlabc123", "source_commit": "commit-one"},
		{"object_id": "c", "source_repository": "scnldef456", "source_commit": "commit-two"},
	}
	for _, doc := range docs {
		if err := checkpoint.do("PUT", "/"+prefix+"objects/_doc/"+doc["object_id"].(string)+"?refresh=true",
			doc, nil); err != nil {
			t.Fatal(err)
		}
	}
	documents, _, err := checkpoint.ownedCounts(prefix)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := checkpoint.capturedSources(documents)
	if err != nil {
		t.Fatalf("source read: %v", err)
	}
	want := []SourceRef{
		{Repository: "scnlabc123", Commit: "commit-one"},
		{Repository: "scnldef456", Commit: "commit-two"},
	}
	if !sameSources(want, sources) {
		t.Fatalf("sources = %v, want %v", sources, want)
	}

	// The same commit under a different repository is a different identity: a
	// commit alone would wrongly consider these interchangeable.
	if sameSources([]SourceRef{{Repository: "scnlabc123", Commit: "commit-one"}},
		[]SourceRef{{Repository: "scnldef456", Commit: "commit-one"}}) {
		t.Fatal("source identity ignored the repository")
	}
}

// TestLiveCapturedSourcesRejectsMissingIdentity fails closed when documents carry
// no source identity: an unidentifiable capture cannot be checked for reuse.
func TestLiveCapturedSourcesRejectsMissingIdentity(t *testing.T) {
	endpoint := os.Getenv("KC_VERIFY_OPENSEARCH_URL")
	repository := os.Getenv("KC_VERIFY_SNAPSHOT_REPO")
	if endpoint == "" || repository == "" {
		t.Skip("live verification requires KC_VERIFY_OPENSEARCH_URL and KC_VERIFY_SNAPSHOT_REPO")
	}
	checkpoint, err := NewSnapshotCheckpoint(endpoint, repository)
	if err != nil {
		t.Fatal(err)
	}
	namespace := "live-nosource"
	prefix := ProjectionPrefix(namespace)
	if err := checkpoint.dropOwned(prefix); err != nil {
		t.Fatalf("pre-clean: %v", err)
	}
	t.Cleanup(func() { _ = checkpoint.dropOwned(prefix) })
	if err := checkpoint.do("PUT", "/"+prefix+"objects", map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": map[string]any{"properties": map[string]any{
			"object_id": map[string]any{"type": "keyword"},
		}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.do("PUT", "/"+prefix+"objects/_doc/a?refresh=true",
		map[string]any{"object_id": "a"}, nil); err != nil {
		t.Fatal(err)
	}
	documents, _, err := checkpoint.ownedCounts(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkpoint.capturedSources(documents); err == nil {
		t.Fatal("source read accepted documents with no source identity")
	} else if !strings.Contains(err.Error(), "source repository") {
		t.Fatalf("unexpected error: %v", err)
	}
}

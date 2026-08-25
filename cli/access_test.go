package cli

import "testing"

func TestAccessExtractionDoesNotInterpretKnowledgePayloadAsEvidence(t *testing.T) {
	result := map[string]any{
		"repository": "kr://acme/knowledge",
		"commit":     "commit-real",
		"knowledgeRef": map[string]any{
			"repository": "kr://acme/knowledge",
			"object":     "Policy:real",
		},
		"value": map[string]any{
			"repository": "kr://payload/not-a-repo",
			"commit":     "payload-version",
			"objectId":   "payload-business-id",
		},
	}
	hits := knowledgeAccesses(result)
	if len(hits) != 1 || hits[0].KnowledgeRef.Object != "Policy:real" {
		t.Fatalf("payload fields became access evidence: %#v", hits)
	}
}

func TestCheckoutExtractionSeparatesSnapshotsFromFiles(t *testing.T) {
	result := map[string]any{"mounts": []any{
		map[string]any{"repository": "kr://acme/source", "commit": "c1", "path": "src", "selector": "refs/heads/main"},
	}}
	if files := fileAccesses(result); len(files) != 0 {
		t.Fatalf("mount paths are not file accesses: %#v", files)
	}
	snapshots := snapshotAccesses(result)
	if len(snapshots) != 1 || snapshots[0].Repository != "kr://acme/source" || snapshots[0].Commit != "c1" {
		t.Fatalf("checkout snapshot evidence missing: %#v", snapshots)
	}
}

func TestSearchResultExtractionUsesHydratedKnowledge(t *testing.T) {
	result := map[string]any{"hits": []any{
		map[string]any{"knowledge": map[string]any{
			"repository": "kr://acme/knowledge",
			"commit":     "commit-search",
			"knowledgeRef": map[string]any{
				"repository": "kr://acme/knowledge",
				"object":     "Metric:gmv",
			},
			"value": map[string]any{"repository": "opaque", "commit": "opaque", "objectId": "opaque"},
		}},
	}}
	hits := knowledgeAccesses(result)
	if len(hits) != 1 || hits[0].KnowledgeRef.Object != "Metric:gmv" || hits[0].KnowledgeRef.Commit != "commit-search" {
		t.Fatalf("hydrated search evidence missing: %#v", hits)
	}
}

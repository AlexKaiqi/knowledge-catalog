package invocation

import (
	"testing"
)

func TestAccessExtractionDoesNotInterpretObjectPayloadAsEvidence(t *testing.T) {
	result := map[string]any{
		"repository": "kr://acme/object",
		"commit":     "commit-real",
		"objectRef": map[string]any{
			"repository": "kr://acme/object",
			"object":     "policy/real",
		},
		"value": map[string]any{
			"repository": "kr://payload/not-a-repo",
			"commit":     "payload-version",
			"objectId":   "payload-business-id",
		},
	}
	hits := ObjectAccesses(result)
	if len(hits) != 1 || hits[0].ObjectRef.Object != "policy/real" {
		t.Fatalf("payload fields became access evidence: %#v", hits)
	}
}

func TestCheckoutExtractionSeparatesSnapshotsFromFiles(t *testing.T) {
	result := map[string]any{"mounts": []any{
		map[string]any{"repository": "kr://acme/source", "commit": "c1", "path": "src", "selector": "refs/heads/main"},
	}}
	if files := FileAccesses(result); len(files) != 0 {
		t.Fatalf("mount paths are not file accesses: %#v", files)
	}
	snapshots := SnapshotAccesses(result)
	if len(snapshots) != 1 || snapshots[0].Repository != "kr://acme/source" || snapshots[0].Commit != "c1" {
		t.Fatalf("checkout snapshot evidence missing: %#v", snapshots)
	}
}

func TestSearchResultExtractionUsesHydratedObject(t *testing.T) {
	result := map[string]any{"hits": []any{
		map[string]any{"object": map[string]any{
			"repository": "kr://acme/object",
			"commit":     "commit-search",
			"objectRef": map[string]any{
				"repository": "kr://acme/object",
				"object":     "metric/gmv",
			},
			"value": map[string]any{"repository": "opaque", "commit": "opaque", "objectId": "opaque"},
		}, "version": map[string]any{"observations": []any{map[string]any{
			"address":           map[string]any{"kind": "Aspect", "objectId": "metric/gmv", "aspectName": "current"},
			"declarationCommit": "commit-search", "declarationDigest": "decl-1",
			"basis": map[string]any{"bindingGeneration": "g1", "consistency": "latest-only", "observedAt": "2026-08-27T13:00:00Z"},
		}}}},
	}}
	hits := ObjectAccesses(result)
	if len(hits) != 1 || hits[0].ObjectRef.Object != "metric/gmv" || hits[0].ObjectRef.Commit != "commit-search" {
		t.Fatalf("hydrated search evidence missing: %#v", hits)
	}
	if len(hits[0].Observations) != 1 || hits[0].Observations[0].Basis.BindingGeneration != "g1" {
		t.Fatalf("search observation evidence missing: %#v", hits)
	}
}

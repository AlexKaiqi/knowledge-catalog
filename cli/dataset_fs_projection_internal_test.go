package cli

import (
	"testing"

	"kc/catalog"
	"kc/kernel"
)

// TestDatasetFSProjectionRejectsPerFileEntries keeps the kcfs projection from
// silently shrinking a recipe: a per-file entry without its mount context can
// only be projected by dropping files, so attach fails loudly instead.
func TestDatasetFSProjectionRejectsPerFileEntries(t *testing.T) {
	projection := datasetFSProjection{
		dataset: "delivery",
		resolved: catalog.ResolvedKnowledgeSet{
			SetID: "delivery", PinID: "p", Items: []catalog.DatasetItem{
				{Kind: catalog.DatasetItemFile, Target: "a.txt", File: "a.txt"},
			},
		},
	}
	_, _, err := projection.build()
	if err == nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("file-system projection must fail loudly on per-file entries instead of shrinking the tree: %v", err)
	}
}

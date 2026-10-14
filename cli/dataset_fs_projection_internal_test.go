package cli

import (
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// TestDatasetFSProjectionRejectsPerFileEntries keeps the kcfs projection from
// silently shrinking a recipe: a per-file entry without its mount context can
// only be projected by dropping files, so attach fails loudly instead.
func TestDatasetFSProjectionRejectsPerFileEntries(t *testing.T) {
	projection := datasetFSProjection{
		dataset: "delivery",
		resolved: dataset.ResolvedDataset{
			SetID: "delivery", PinID: "p", Items: []dataset.DatasetItem{
				{Kind: dataset.DatasetItemFile, Target: "a.txt", File: "a.txt"},
			},
		},
	}
	_, _, err := projection.build()
	if err == nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("file-system projection must fail loudly on per-file entries instead of shrinking the tree: %v", err)
	}
}

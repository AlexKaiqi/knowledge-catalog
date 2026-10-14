package invocation

import (
	"encoding/json"
	"kc/kernel"
	"kc/snapshot/dataset"
	"testing"
)

func TestDatasetRegressionReplayCannotBroadenDatasetItems(t *testing.T) {
	id := kernel.RepositoryID("kr://acme/source")
	def := dataset.Dataset{SetID: "public-only", Revision: 1, Sources: []dataset.DatasetSource{{Repository: id, Selector: "refs/heads/main", Commit: "published", SubPath: "public"}}}
	pin := dataset.ResolvedDataset{SetID: def.SetID, Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{id: "published"},
		Items: []dataset.DatasetItem{{Repository: id, Commit: "published", Kind: dataset.DatasetItemPrefix, Prefix: ""}}}
	pin.PinID = dataset.HashResolved(def.SetID, def.Sources, pin.Repositories)
	raw, _ := json.Marshal(pin)
	got, err := DecodeReplayPin(def, string(raw))
	if err == nil && dataset.DatasetPathAllowed(got.Items, id, "private/secret.yaml") {
		t.Fatal("authentic scope hash accepted caller-supplied whole-repository Items")
	}
}

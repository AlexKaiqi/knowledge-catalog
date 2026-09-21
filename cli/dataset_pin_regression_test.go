package cli

import (
	"encoding/json"
	"kc/catalog"
	"kc/kernel"
	"testing"
)

func TestDatasetRegressionReplayCannotBroadenDatasetItems(t *testing.T) {
	id := kernel.RepositoryID("kr://acme/source")
	def := catalog.KnowledgeSet{SetID: "public-only", Revision: 1, Sources: []catalog.KnowledgeSetSource{{Repository: id, Selector: "refs/heads/main", Commit: "published", SubPath: "public"}}}
	pin := catalog.ResolvedKnowledgeSet{SetID: def.SetID, Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{id: "published"},
		Items: []catalog.DatasetItem{{Repository: id, Commit: "published", Kind: catalog.DatasetItemPrefix, Prefix: ""}}}
	pin.PinID = catalog.HashResolved(def.SetID, def.Sources, pin.Repositories)
	raw, _ := json.Marshal(pin)
	got, err := decodeReplayPin(def, string(raw))
	if err == nil && catalog.DatasetPathAllowed(got.Items, id, "private/secret.yaml") {
		t.Fatal("authentic scope hash accepted caller-supplied whole-repository Items")
	}
}

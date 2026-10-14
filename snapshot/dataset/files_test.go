package dataset_test

import (
	"testing"

	catalog "kc/snapshot/dataset"
)

func TestWorkspaceJSONDoesNotAcceptLegacyCompositionKeys(t *testing.T) {
	var pin catalog.ResolvedDataset
	if err := catalog.DecodeJSON([]byte(`{"setId":"agent","revision":1,"repositories":{}}`), &pin); err != nil {
		t.Fatal(err)
	}
	if pin.SetID != "agent" {
		t.Fatal(pin)
	}
	if err := catalog.DecodeJSON([]byte(`{"viewId":"agent","revision":1,"repositories":{}}`), &pin); err == nil {
		t.Fatal("legacy viewId must not be accepted as a Workspace pin")
	}
}

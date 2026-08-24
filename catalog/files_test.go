package catalog_test

import (
	"testing"

	"kc/catalog"
)

func TestWorkspaceJSONDoesNotAcceptLegacyCompositionKeys(t *testing.T) {
	var pin catalog.ResolvedWorkspace
	if err := catalog.DecodeJSON([]byte(`{"workspaceId":"agent","revision":1,"repositories":{}}`), &pin); err != nil {
		t.Fatal(err)
	}
	if pin.WorkspaceID != "agent" {
		t.Fatal(pin)
	}
	if err := catalog.DecodeJSON([]byte(`{"viewId":"agent","revision":1,"repositories":{}}`), &pin); err == nil {
		t.Fatal("legacy viewId must not be accepted as a Workspace pin")
	}
}

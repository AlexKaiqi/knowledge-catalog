package invocation

import (
	apphome "kc/home"
	"kc/invoke"
	"kc/kernel"
	"strings"
	"testing"
)

func TestInventoryDiscoveryContextRequiresExactPublishedWorkspaceAndAction(t *testing.T) {
	ws := &apphome.Home{Deployment: &apphome.DeploymentConfig{Registry: apphome.RegistryBinding{DiscoverySetID: "published"}}}
	flags := map[string]invoke.FlagValue{"dataset": "published", "pin": "{}", InventoryDiscoveryFlag: true}
	if err := PrepareInventoryDiscovery(ws, "knowledge.search", flags); err != nil {
		t.Fatal(err)
	}
	if !IsInventoryDiscovery(flags) {
		t.Fatal("validated context not established")
	}
	for _, action := range []string{"knowledge.read", "knowledge.rerank", "file.read", "dataset.manage"} {
		if err := PrepareInventoryDiscovery(ws, action, flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Errorf("discovery context bypassed %s: %v", action, err)
		}
	}
	flags["dataset"] = "private"
	if err := PrepareInventoryDiscovery(ws, "knowledge.search", flags); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("unconfigured Workspace accepted %v", err)
	}
	flags["dataset"] = "published"
	delete(flags, "pin")
	if err := PrepareInventoryDiscovery(ws, "knowledge.search", flags); err == nil || !strings.Contains(err.Error(), "pin") {
		t.Fatalf("search without fixed pin accepted %v", err)
	}
}

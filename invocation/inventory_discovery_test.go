package invocation

import (
	apphome "kc/home"
	"kc/invoke"
	"kc/kernel"
	"testing"
)

func TestRetiredInventoryDiscoveryNeverCreatesAnAuthorizationContext(t *testing.T) {
	ws := &apphome.Home{Deployment: &apphome.DeploymentConfig{Registry: apphome.RegistryBinding{DiscoverySetID: "published"}}}
	for _, action := range []string{"object.search", "file.read", "dataset.resolve", "object.read", "object.rerank", "dataset.manage"} {
		flags := map[string]invoke.FlagValue{"dataset": "published", "pin": "{}", InventoryDiscoveryFlag: true, VerifiedInventoryDiscoveryFlag: InventoryDiscoveryContext{"published"}}
		if err := PrepareInventoryDiscovery(ws, action, flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid || IsInventoryDiscovery(flags) {
			t.Fatalf("retired inventory context bypassed %s: flags=%#v err=%v", action, flags, err)
		}
	}
	flags := map[string]invoke.FlagValue{"dataset": "published"}
	if err := PrepareInventoryDiscovery(ws, "file.read", flags); err != nil || IsInventoryDiscovery(flags) {
		t.Fatalf("ordinary Dataset request changed: %v", err)
	}
}

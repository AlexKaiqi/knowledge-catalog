package invocation

import (
	"kc/home"
	"kc/invoke"
	"kc/kernel"
)

const InventoryDiscoveryFlag = "_inventory-discovery"
const VerifiedInventoryDiscoveryFlag = "_verified-inventory-discovery"

// InventoryDiscoveryContext is the typed marker a Server stores on a request
// after matching the deployment's configured discovery Workspace.
type InventoryDiscoveryContext struct{ workspace string }

func ConfiguredDiscoveryWorkspace(ws *home.Home) string {
	if ws == nil || ws.Deployment == nil {
		return ""
	}
	return ws.Deployment.Registry.DiscoverySetID
}

// PrepareInventoryDiscovery is the request-context verification behind the
// inventoryDiscovery request flag. The public boolean expresses a request
// context, never authority: only the Server can establish the typed marker,
// after matching the deployment's configured discovery Workspace.
func PrepareInventoryDiscovery(_ *home.Home, _ string, flags map[string]invoke.FlagValue) error {
	delete(flags, VerifiedInventoryDiscoveryFlag)
	if flags[InventoryDiscoveryFlag] == true {
		return kernel.Fail(kernel.ErrUsageInvalid, "inventoryDiscovery is retired; select a Dataset from inventory and use its consumption permission")
	}
	return nil
}

// isInventoryDiscovery reports the verified marker for the authorization gate;
// only Authorize consumes it.
func IsInventoryDiscovery(flags map[string]invoke.FlagValue) bool {
	context, ok := flags[VerifiedInventoryDiscoveryFlag].(InventoryDiscoveryContext)
	return ok && context.workspace == invoke.FlagString(flags, "dataset")
}

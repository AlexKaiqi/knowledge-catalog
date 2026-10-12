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
func PrepareInventoryDiscovery(ws *home.Home, action string, flags map[string]invoke.FlagValue) error {
	delete(flags, VerifiedInventoryDiscoveryFlag)
	if flags[InventoryDiscoveryFlag] != true {
		return nil
	}
	if action != "dataset.resolve" && action != "knowledge.search" {
		return kernel.Fail(kernel.ErrUsageInvalid, "inventoryDiscovery is only valid for ResolveKnowledgeSet and SEARCH")
	}
	workspace := invoke.FlagString(flags, "dataset")
	if workspace == "" || invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" || SuppliedKnowledgeSet(flags) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "inventoryDiscovery requires the configured published Dataset")
	}
	configured := ConfiguredDiscoveryWorkspace(ws)
	if configured == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "inventory has no configured discoveryWorkspaceId")
	}
	if workspace != configured {
		return kernel.Fail(kernel.ErrForbidden, "Workspace is not the configured inventory discovery Workspace")
	}
	if action == "knowledge.search" && invoke.FlagString(flags, "pin") == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "inventory discovery SEARCH requires a fixed pin from ResolveKnowledgeSet")
	}
	flags[VerifiedInventoryDiscoveryFlag] = InventoryDiscoveryContext{workspace}
	return nil
}

// isInventoryDiscovery reports the verified marker for the authorization gate;
// only Authorize consumes it.
func IsInventoryDiscovery(flags map[string]invoke.FlagValue) bool {
	context, ok := flags[VerifiedInventoryDiscoveryFlag].(InventoryDiscoveryContext)
	return ok && context.workspace == invoke.FlagString(flags, "dataset")
}

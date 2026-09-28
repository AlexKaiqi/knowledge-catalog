package invocation

import (
	"kc/home"
	"kc/invoke"
	"kc/kernel"
)

const CatalogDiscoveryFlag = "_catalog-discovery"
const VerifiedCatalogDiscoveryFlag = "_verified-catalog-discovery"

// CatalogDiscoveryContext is the typed marker a Server stores on a request
// after matching the deployment's configured discovery Workspace.
type CatalogDiscoveryContext struct{ catalog, workspace string }

func ConfiguredDiscoveryWorkspace(ws *home.Home, catalogID string) string {
	if ws == nil || ws.Deployment == nil {
		return ""
	}
	for _, binding := range ws.Deployment.Catalogs {
		if binding.ID == catalogID {
			return binding.DiscoverySetID
		}
	}
	return ""
}

// PrepareCatalogDiscovery is the request-context verification behind the
// catalogDiscovery request flag. The public boolean expresses a request
// context, never authority: only the Server can establish the typed marker,
// after matching the deployment's configured discovery Workspace.
func PrepareCatalogDiscovery(ws *home.Home, action string, flags map[string]invoke.FlagValue) error {
	delete(flags, VerifiedCatalogDiscoveryFlag)
	if flags[CatalogDiscoveryFlag] != true {
		return nil
	}
	if action != "dataset.resolve" && action != "knowledge.search" {
		return kernel.Fail(kernel.ErrUsageInvalid, "catalogDiscovery is only valid for ResolveKnowledgeSet and SEARCH")
	}
	catalogID, workspace := invoke.FlagString(flags, "catalog"), invoke.FlagString(flags, "dataset")
	if catalogID == "" || workspace == "" || invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" || SuppliedKnowledgeSet(flags) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "catalogDiscovery requires an explicit Catalog and its published Workspace")
	}
	configured := ConfiguredDiscoveryWorkspace(ws, catalogID)
	if configured == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Catalog has no configured discoveryWorkspaceId")
	}
	if workspace != configured {
		return kernel.Fail(kernel.ErrForbidden, "Workspace is not the configured Catalog discovery Workspace")
	}
	if action == "knowledge.search" && invoke.FlagString(flags, "pin") == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "Catalog discovery SEARCH requires a fixed pin from ResolveKnowledgeSet")
	}
	flags[VerifiedCatalogDiscoveryFlag] = CatalogDiscoveryContext{catalogID, workspace}
	return nil
}

// isCatalogDiscovery reports the verified marker for the authorization gate;
// only Authorize consumes it.
func IsCatalogDiscovery(flags map[string]invoke.FlagValue) bool {
	context, ok := flags[VerifiedCatalogDiscoveryFlag].(CatalogDiscoveryContext)
	return ok && context.catalog == invoke.FlagString(flags, "catalog") && context.workspace == invoke.FlagString(flags, "dataset")
}

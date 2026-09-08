package cli

import (
	"context"
	"encoding/json"

	"kc/catalog"
	kcclient "kc/client"
	"kc/kernel"
)

const catalogDiscoveryFlag = "_catalog-discovery"
const verifiedCatalogDiscoveryFlag = "_verified-catalog-discovery"

type catalogDiscoveryContext struct{ catalog, workspace string }

func configuredDiscoveryWorkspace(ws *Home, catalogID string) string {
	if ws == nil || ws.Deployment == nil {
		return ""
	}
	for _, binding := range ws.Deployment.Catalogs {
		if binding.ID == catalogID {
			return binding.DiscoveryWorkspaceID
		}
	}
	return ""
}

// The public boolean expresses a request context, never authority. Only the
// Server can establish this typed marker after matching deployment selection.
func prepareCatalogDiscovery(ws *Home, action string, flags map[string]FlagValue) error {
	delete(flags, verifiedCatalogDiscoveryFlag)
	if flags[catalogDiscoveryFlag] != true {
		return nil
	}
	if action != "workspace.resolve" && action != "knowledge.search" {
		return kernel.Fail(kernel.ErrUsageInvalid, "catalogDiscovery is only valid for ResolveWorkspace and SEARCH")
	}
	catalogID, workspace := FlagString(flags, "catalog"), FlagString(flags, "workspace")
	if catalogID == "" || workspace == "" || FlagString(flags, "repo") != "" || FlagString(flags, "commit") != "" || FlagString(flags, "ref") != "" || suppliedWorkspaceDefinition(flags) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "catalogDiscovery requires an explicit Catalog and its published Workspace")
	}
	configured := configuredDiscoveryWorkspace(ws, catalogID)
	if configured == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Catalog has no configured discoveryWorkspaceId")
	}
	if workspace != configured {
		return kernel.Fail(kernel.ErrForbidden, "Workspace is not the configured Catalog discovery Workspace")
	}
	if action == "knowledge.search" && FlagString(flags, "pin") == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "Catalog discovery SEARCH requires a fixed pin from ResolveWorkspace")
	}
	flags[verifiedCatalogDiscoveryFlag] = catalogDiscoveryContext{catalogID, workspace}
	return nil
}

func isCatalogDiscovery(flags map[string]FlagValue) bool {
	context, ok := flags[verifiedCatalogDiscoveryFlag].(catalogDiscoveryContext)
	return ok && context.catalog == FlagString(flags, "catalog") && context.workspace == FlagString(flags, "workspace")
}

func catalogSearchRequested(path string, flags map[string]FlagValue) bool {
	if path != "knowledge search" || FlagString(flags, "catalog") == "" {
		return false
	}
	for _, name := range []string{"repo", "workspace", "pin", "source", "workspace-file", "file", "commit", "ref"} {
		if FlagString(flags, name) != "" {
			return false
		}
	}
	return suppliedWorkspaceDefinition(flags) == nil
}

func prepareRemoteCatalogDiscovery(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) error {
	catalogID := FlagString(flags, "catalog")
	var view struct {
		DiscoveryWorkspaceID string `json:"discoveryWorkspaceId"`
	}
	if err := client.CatalogService().Show(ctx, catalogID, options, &view); err != nil {
		return err
	}
	if view.DiscoveryWorkspaceID == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Catalog has no configured discoveryWorkspaceId")
	}
	var pin catalog.ResolvedWorkspace
	if err := client.CatalogService().ResolveWorkspace(ctx, catalogID, view.DiscoveryWorkspaceID, kcclient.WorkspaceResolveRequest{CatalogDiscovery: true}, options, &pin); err != nil {
		return err
	}
	raw, err := json.Marshal(pin)
	if err != nil {
		return err
	}
	flags["workspace"] = view.DiscoveryWorkspaceID
	flags["pin"] = string(raw)
	return nil
}

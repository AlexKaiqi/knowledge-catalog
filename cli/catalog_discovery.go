package cli

import (
	"context"
	"encoding/json"

	"kc/catalog"
	kcclient "kc/client"
	"kc/kernel"
)

// The catalog-discovery request/verification vocabulary (CatalogDiscoveryFlag,
// the typed verified marker, PrepareCatalogDiscovery, and the workspace
// lookup) moved to kc/invocation in CAP-ORG-01 B5b: the shared authorization
// gate consumes the marker, so it lives in the execution seam. This file keeps
// the CLI-side discovery flows: the remote resolution path (which must go
// through the typed client) and the search-shape check.

// catalogSearchRequested reports whether an argv search is a bare Catalog
// discovery search (no repo/dataset/pin scoping).
func catalogSearchRequested(path string, flags map[string]FlagValue) bool {
	if path != "search" || FlagString(flags, "catalog") == "" {
		return false
	}
	for _, name := range []string{"repo", "dataset", "pin", "source", "dataset-file", "file", "commit", "ref"} {
		if FlagString(flags, name) != "" {
			return false
		}
	}
	return suppliedKnowledgeSet(flags) == nil
}

func prepareRemoteCatalogDiscovery(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) error {
	catalogID := FlagString(flags, "catalog")
	var view struct {
		DiscoverySetID string `json:"discoveryWorkspaceId"`
	}
	if err := client.CatalogService().Show(ctx, catalogID, options, &view); err != nil {
		return err
	}
	if view.DiscoverySetID == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Catalog has no configured discoveryWorkspaceId")
	}
	var pin catalog.ResolvedKnowledgeSet
	if err := client.CatalogService().ResolveKnowledgeSet(ctx, catalogID, view.DiscoverySetID, kcclient.KnowledgeSetResolveRequest{CatalogDiscovery: true}, options, &pin); err != nil {
		return err
	}
	raw, err := json.Marshal(pin)
	if err != nil {
		return err
	}
	flags["dataset"] = view.DiscoverySetID
	flags["pin"] = string(raw)
	return nil
}

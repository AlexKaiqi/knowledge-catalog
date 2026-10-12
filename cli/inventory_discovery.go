package cli

import (
	"context"
	"encoding/json"

	kcclient "kc/client"
	"kc/kernel"
	"kc/snapshot/dataset"
)

// The inventory-discovery request/verification vocabulary (InventoryDiscoveryFlag,
// the typed verified marker, PrepareInventoryDiscovery, and the workspace
// lookup) moved to kc/invocation in CAP-ORG-01 B5b: the shared authorization
// gate consumes the marker, so it lives in the execution seam. This file keeps
// the CLI-side discovery flows: the remote resolution path (which must go
// through the typed client) and the search-shape check.

// inventorySearchRequested reports whether an argv search is a bare inventory
// discovery search (no repo/dataset/pin scoping).
func inventorySearchRequested(path string, flags map[string]FlagValue) bool {
	if path != "search" {
		return false
	}
	for _, name := range []string{"repo", "dataset", "pin", "source", "dataset-file", "file", "commit", "ref"} {
		if flagString(flags, name) != "" {
			return false
		}
	}
	return suppliedKnowledgeSet(flags) == nil
}

func prepareRemoteInventoryDiscovery(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) error {
	var view struct {
		DiscoverySetID string `json:"discoveryWorkspaceId"`
	}
	if err := client.SnapshotService().Show(ctx, options, &view); err != nil {
		return err
	}
	if view.DiscoverySetID == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "service has no configured discoveryWorkspaceId")
	}
	var pin dataset.ResolvedKnowledgeSet
	if err := client.SnapshotService().ResolveKnowledgeSet(ctx, view.DiscoverySetID, kcclient.KnowledgeSetResolveRequest{InventoryDiscovery: true}, options, &pin); err != nil {
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

package cli

import (
	"context"
	"strconv"

	kcclient "kc/client"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	snapshotclient "kc/snapshot/client"
	"kc/snapshot/dataset"
)

// runRemoteTaskResolution preserves the typed task adapter used by application
// contracts. Public argv and module Commands do not register these task names.
func runRemoteTaskResolution(ctx context.Context, remote *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	service := remote.SnapshotService()
	requestOptions := snapshotRequestOptions(options)
	id := invoke.FlagString(flags, "dataset")
	if path == "pin check" {
		var output any
		err := service.CheckDataset(ctx, id, snapshotclient.DatasetResolveRequest{Pin: remotePin(flags)}, requestOptions, &output)
		return output, err
	}
	var resolved dataset.ResolvedDataset
	var definition *dataset.Dataset
	var err error
	if id != "" {
		err = service.ResolveDataset(ctx, id, snapshotclient.DatasetResolveRequest{Pin: remotePin(flags)}, requestOptions, &resolved)
	} else {
		definition, err = taskResolutionDefinition(flags)
		if err == nil {
			err = service.ResolveDefinition(ctx, snapshotclient.DatasetRequest{Revision: definition.Revision, Sources: definition.Sources, Metadata: definition.Metadata}, requestOptions, &resolved)
		}
	}
	if err != nil {
		return nil, err
	}
	return inv.ShapePinOutput(flags, inv.TaskDatasetPin{ResolvedDataset: resolved, Definition: definition})
}

func taskResolutionDefinition(flags map[string]FlagValue) (*dataset.Dataset, error) {
	if invoke.FlagString(flags, "from-repo") != "" {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote Workspace definition does not read a server repository recipe; submit explicit sources")
	}
	sources, recipe, _, err := inv.WorkspaceSources(&inv.Invocation{Flags: flags})
	if err != nil {
		return nil, err
	}
	metadata, err := inv.DatasetMetadata(flags, recipe.Metadata)
	if err != nil {
		return nil, err
	}
	revision := 1
	if raw := invoke.FlagString(flags, "revision"); raw != "" {
		raw, err = inv.RequireRemoteFlag(flags, "revision")
		if err != nil {
			return nil, err
		}
		revision, err = strconv.Atoi(raw)
		if err != nil {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "--revision must be an integer")
		}
	}
	return &dataset.Dataset{Revision: revision, Sources: sources, Metadata: metadata}, nil
}

// Retired task adapters are used only by the application contract harness.
func runFixtureRemoteRequest(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if path == "pin" || path == "pin check" {
		return runRemoteTaskResolution(ctx, client, path, flags, options)
	}
	return runRemoteRequest(ctx, client, path, flags, options)
}

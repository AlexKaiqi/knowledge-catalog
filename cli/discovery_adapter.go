package cli

import (
	"context"
	kcclient "kc/client"
	discoverycli "kc/discovery/cli"
	discoveryclient "kc/discovery/client"
	"kc/invoke"

	inv "kc/invocation"
)

// The host assembles module-owned declarations and supplies authenticated SDKs.

var discoveryCLIEntries = discoverycli.Commands()

func discoveryCommandByHandler(handler string) (discoverycli.Command, bool) {
	return moduleCommandByHandler(discoveryCLIEntries, handler)
}
func discoveryCommandPath(path string) bool {
	return moduleCommandPath(discoveryCLIEntries, path)
}
func bindDiscoveryOperands(entry discoverycli.Command, flags map[string]FlagValue, args []string) error {
	return bindModuleOperands(entry.Operands, flags, args)
}
func discoveryHelpEntries() map[string]helpEntry {
	return moduleHelpEntries(discoveryCLIEntries)
}
func composeDiscoveryHelpGroups(host map[string][]string) map[string][]string {
	return moduleHelpGroups(discoveryCLIEntries, host)
}
func includeDiscoveryValueHelp(host []string) []string {
	return moduleValueHelp(discoveryCLIEntries, host)
}
func composeDiscoveryCLICommands(entries []discoverycli.Command, host map[string]commandSurface) map[string]commandSurface {
	return composeModuleCommands(entries, host)
}

func discoveryCaller(flags map[string]FlagValue) discoverycli.Context {
	return discoverycli.Context{Dataset: invoke.FlagString(flags, "dataset"), Pin: remotePin(flags), Definition: inv.SuppliedKnowledgeSet(flags)}
}

func discoveryFlags(flags map[string]FlagValue) map[string]any {
	values := snapshotFlags(flags)
	delete(values, "pin") // Fixed coordinates are caller context, not new public flags.
	return values
}

func validateDiscoveryFlags(path string, flags map[string]FlagValue) error {
	return discoverycli.New(nil, discoverycli.Context{}).ValidateFlags(path, snapshotFlags(flags))
}

func preflightDiscoveryClient(path string, flags map[string]FlagValue) error {
	return discoverycli.New(nil, discoverycli.Context{Dataset: invoke.FlagString(flags, "dataset")}).Validate(path, snapshotFlags(flags))
}

func executeDiscoveryCommand(ctx context.Context, remote *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if path == "plugin" {
		return discoverycli.New(remote.DiscoveryService(), discoverycli.Context{}).Execute(ctx, path, snapshotFlags(flags), discoveryclient.RequestOptions{RequestID: options.RequestID})
	}
	inventory := inventorySearchRequested(path, flags)
	if err := inv.PrepareKnowledgePinContext(flags); err != nil {
		return nil, err
	}
	if inventory {
		if err := prepareRemoteInventoryDiscovery(ctx, remote, flags, options); err != nil {
			return nil, err
		}
	}
	caller := discoveryCaller(flags)
	caller.InventoryDiscovery = inventory
	return discoverycli.New(remote.DiscoveryService(), caller).Execute(ctx, path, discoveryFlags(flags), discoveryclient.RequestOptions{RequestID: options.RequestID})
}

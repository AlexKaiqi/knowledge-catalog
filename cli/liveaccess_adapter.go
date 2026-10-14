package cli

import (
	"context"
	kcclient "kc/client"
	inv "kc/invocation"
	"kc/invoke"
	accesscli "kc/liveaccess/cli"
	accessclient "kc/liveaccess/client"
	objectclient "kc/object/client"
)

var liveAccessCLIEntries = accesscli.Commands()

func liveAccessCommandByHandler(handler string) (accesscli.Command, bool) {
	return moduleCommandByHandler(liveAccessCLIEntries, handler)
}
func liveAccessCommandPath(path string) bool      { return moduleCommandPath(liveAccessCLIEntries, path) }
func liveAccessHelpEntries() map[string]helpEntry { return moduleHelpEntries(liveAccessCLIEntries) }
func composeLiveAccessHelpGroups(host map[string][]string) map[string][]string {
	return moduleHelpGroups(liveAccessCLIEntries, host)
}
func includeLiveAccessValueHelp(host []string) []string {
	return moduleValueHelp(liveAccessCLIEntries, host)
}
func composeLiveAccessCLICommands(entries []accesscli.Command, host map[string]commandSurface) map[string]commandSurface {
	return composeModuleCommands(entries, host)
}
func validateLiveAccessFlags(path string, flags map[string]FlagValue) error {
	return accesscli.New(nil, accesscli.Context{}).ValidateFlags(path, snapshotFlags(flags))
}
func preflightLiveAccessClient(path string, flags map[string]FlagValue) error {
	if !liveAccessCommandPath(path) {
		return nil
	}
	return accesscli.New(nil, accesscli.Context{Scope: objectclient.KnowledgeScope{Dataset: invoke.FlagString(flags, "dataset")}}).Validate(path, snapshotFlags(flags))
}
func executeLiveAccessCommand(ctx context.Context, remote *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	if err := inv.PrepareKnowledgePinContext(flags); err != nil {
		return nil, err
	}
	scope := objectclient.KnowledgeScope{Dataset: invoke.FlagString(flags, "dataset"), Pin: remotePin(flags), Definition: inv.SuppliedKnowledgeSet(flags)}
	return accesscli.New(remote.LiveAccessService(), accesscli.Context{Scope: scope}).Execute(ctx, path, accesscli.InputFlags(path, snapshotFlags(flags)), accessclient.RequestOptions{RequestID: options.RequestID})
}

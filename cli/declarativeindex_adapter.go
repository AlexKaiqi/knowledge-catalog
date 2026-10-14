package cli

import (
	"context"
	kcclient "kc/client"
	declarativeindexcli "kc/declarativeindex/cli"
	declarativeindexclient "kc/declarativeindex/client"
)

// The host assembles module-owned declarations and supplies authenticated SDKs.
var declarativeIndexCLIEntries = declarativeindexcli.Commands()

func declarativeIndexCommandByHandler(handler string) (declarativeindexcli.Command, bool) {
	return moduleCommandByHandler(declarativeIndexCLIEntries, handler)
}
func declarativeIndexCommandPath(path string) bool {
	return moduleCommandPath(declarativeIndexCLIEntries, path)
}
func bindDeclarativeIndexOperands(entry declarativeindexcli.Command, flags map[string]FlagValue, args []string) error {
	return bindModuleOperands(entry.Operands, flags, args)
}
func declarativeIndexHelpEntries() map[string]helpEntry {
	return moduleHelpEntries(declarativeIndexCLIEntries)
}
func composeDeclarativeIndexHelpGroups(host map[string][]string) map[string][]string {
	return moduleHelpGroups(declarativeIndexCLIEntries, host)
}
func includeDeclarativeIndexValueHelp(host []string) []string {
	return moduleValueHelp(declarativeIndexCLIEntries, host)
}
func composeDeclarativeIndexCLICommands(entries []declarativeindexcli.Command, host map[string]commandSurface) map[string]commandSurface {
	return composeModuleCommands(entries, host)
}

func validateDeclarativeIndexFlags(path string, flags map[string]FlagValue) error {
	return declarativeindexcli.New(nil).ValidateFlags(path, snapshotFlags(flags))
}

func preflightDeclarativeIndexClient(path string, flags map[string]FlagValue) error {
	return declarativeindexcli.New(nil).Validate(path, snapshotFlags(flags))
}

func executeDeclarativeIndexCommand(ctx context.Context, remote *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	return declarativeindexcli.New(remote.DeclarativeIndexService()).Execute(ctx, path, snapshotFlags(flags), declarativeindexclient.RequestOptions{RequestID: options.RequestID})
}

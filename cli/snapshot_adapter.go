package cli

import (
	"context"
	"strings"

	kcclient "kc/client"
	snapshotcli "kc/snapshot/cli"
	snapshotclient "kc/snapshot/client"

	"kc/invoke"
)

// The host binds and displays the module's declarations. Leaf parameters,
// descriptions and product results are owned by snapshot/cli.

var snapshotCLIEntries = snapshotcli.Commands()

var snapshotHelpTopics = snapshotcli.HelpTopics()

func snapshotCommandByHandler(handler string) (snapshotcli.Command, bool) {
	return moduleCommandByHandler(snapshotCLIEntries, handler)
}

func snapshotCommandPath(path string) bool {
	return moduleCommandPath(snapshotCLIEntries, path)
}

func bindSnapshotOperands(entry snapshotcli.Command, flags map[string]FlagValue, args []string) error {
	return bindModuleOperands(entry.Operands, flags, args)
}

func snapshotHelpEntries() map[string]helpEntry {
	return moduleHelpEntries(snapshotCLIEntries)
}

func composeSnapshotHelpGroups(host map[string][]string) map[string][]string {
	return moduleHelpGroups(snapshotCLIEntries, host)
}

func includeSnapshotValueHelp(host []string) []string {
	return moduleValueHelp(snapshotCLIEntries, host)
}

func validateSnapshotFlags(path string, flags map[string]FlagValue) error {
	return snapshotcli.New(nil, snapshotcli.Context{}).ValidateFlags(path, snapshotFlags(flags))
}

func preflightSnapshotClient(path string, flags map[string]FlagValue) error {
	return snapshotcli.New(nil, snapshotcli.Context{}).Validate(path, snapshotFlags(flags))
}

func snapshotFlags(flags map[string]FlagValue) map[string]any {
	values := make(map[string]any, len(flags))
	for name, value := range flags {
		if strings.HasPrefix(name, "_") {
			continue
		}
		values[name] = value
	}
	return values
}

func snapshotRequestOptions(options kcclient.RequestOptions) snapshotclient.RequestOptions {
	return snapshotclient.RequestOptions{RequestID: options.RequestID}
}

func executeSnapshotCommand(ctx context.Context, remote *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var caller snapshotcli.Context
	if path == "grant list" && invoke.FlagString(flags, "action") != "" && invoke.FlagString(flags, "principal") == "" {
		identity, _, err := remote.Identity(ctx)
		if err != nil {
			return nil, err
		}
		caller.Principal = identity.Principal
	}
	return snapshotcli.New(remote, caller).Execute(ctx, path, snapshotFlags(flags), snapshotRequestOptions(options))
}

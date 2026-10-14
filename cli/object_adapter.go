package cli

import (
	"context"
	kcclient "kc/client"
	objectcli "kc/object/cli"
	objectclient "kc/object/client"
	"os"
	"strings"
)

// Object contributes its leaves, root model help and result view. The host
// only binds argv, supplies the authenticated SDK, and composes contributions.
var objectCLIEntries = objectcli.Commands()

func objectCommandByHandler(handler string) (objectcli.Command, bool) {
	return moduleCommandByHandler(objectCLIEntries, handler)
}
func objectCommandPath(path string) bool {
	return moduleCommandPath(objectCLIEntries, path)
}
func bindObjectOperands(entry objectcli.Command, flags map[string]FlagValue, args []string) error {
	values := snapshotFlags(flags)
	if err := objectcli.BindOperands(entry.Path, values, args); err != nil {
		return err
	}
	for name, value := range values {
		flags[name] = value
	}
	return nil
}
func objectHelpEntries() map[string]helpEntry {
	return moduleHelpEntries(objectCLIEntries)
}
func composeObjectHelpGroups(host map[string][]string) map[string][]string {
	return moduleHelpGroups(objectCLIEntries, host)
}
func includeObjectValueHelp(host []string) []string {
	return moduleValueHelp(objectCLIEntries, host)
}
func composeObjectCLICommands(entries []objectcli.Command, host map[string]commandSurface) map[string]commandSurface {
	return composeModuleCommands(entries, host)
}
func objectCaller() objectcli.Context {
	return objectcli.Context{Dataset: strings.TrimSpace(os.Getenv("KC_DATASET"))}
}
func validateObjectFlags(path string, flags map[string]FlagValue) error {
	return objectcli.New(nil, objectCaller()).ValidateFlags(path, snapshotFlags(flags))
}
func preflightObjectClient(path string, flags map[string]FlagValue) error {
	return objectcli.New(nil, objectCaller()).Validate(path, snapshotFlags(flags))
}
func executeObjectCommand(ctx context.Context, remote *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	return objectcli.New(remote.ObjectService(), objectCaller()).Execute(ctx, path, snapshotFlags(flags), objectclient.RequestOptions{RequestID: options.RequestID})
}

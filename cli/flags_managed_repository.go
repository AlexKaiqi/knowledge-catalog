package cli

import (
	inv "kc/invocation"
	"kc/kernel"
)

var managedRepositoryCreateFlags = inv.FlagNames("name store url credential-file server as request-id trace-id span-id parent-span-id help")

func validateManagedRepositoryCreateFlags(flags map[string]FlagValue) error {
	if err := rejectFlagsOutside(flags, managedRepositoryCreateFlags, "kc create"); err != nil {
		return err
	}
	name := flagString(flags, "name")
	connectionURL := flagString(flags, "url")
	if (name == "") == (connectionURL == "") {
		return kernel.Fail(kernel.ErrUsageInvalid, "kc create requires exactly one of --name or --url")
	}
	if name != "" {
		if flagString(flags, "credential-file") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "kc create --name does not accept --credential-file")
		}
		return nil
	}
	if flagString(flags, "store") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "kc create --url does not accept --store")
	}
	if flagString(flags, "credential-file") == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "kc create --url requires --credential-file")
	}
	return nil
}

package cli

import (
	"fmt"

	"kc/kernel"
)

var managedRepositoryCreateFlags = flagNames("name store url credential-file server as request-id trace-id span-id parent-span-id help")

func validateManagedRepositoryCreateFlags(flags map[string]FlagValue) error {
	if err := rejectFlagsOutside(flags, managedRepositoryCreateFlags, "kc create"); err != nil {
		return err
	}
	name := FlagString(flags, "name")
	connectionURL := FlagString(flags, "url")
	if (name == "") == (connectionURL == "") {
		return kernel.Fail(kernel.ErrUsageInvalid, "kc create requires exactly one of --name or --url")
	}
	if name != "" {
		if FlagString(flags, "credential-file") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "kc create --name does not accept --credential-file")
		}
		return nil
	}
	if FlagString(flags, "store") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "kc create --url does not accept --store")
	}
	if FlagString(flags, "credential-file") == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "kc create --url requires --credential-file")
	}
	return nil
}

func validateManagedRepositoryCoordinates(flags map[string]FlagValue) error {
	if FlagString(flags, "name") != "" {
		if FlagString(flags, "repo") != "" || FlagString(flags, "command-id") != "" {
			return fmt.Errorf("--name cannot be combined with --repo or --command-id; LakeFS uses --name as the repository id")
		}
		return nil
	}
	for _, name := range []string{"catalog", "repo", "command-id"} {
		if _, err := requireRemoteFlag(flags, name); err != nil {
			return err
		}
	}
	return nil
}

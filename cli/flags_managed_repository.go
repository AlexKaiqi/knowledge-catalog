package cli

var managedRepositoryCreateFlags = flagNames("catalog repo command-id server as request-id trace-id span-id parent-span-id help")

func validateManagedRepositoryCreateFlags(flags map[string]FlagValue) error {
	if err := rejectFlagsOutside(flags, managedRepositoryCreateFlags, "kc catalog repo create"); err != nil {
		return err
	}
	return validateManagedRepositoryCoordinates(flags)
}

func validateManagedRepositoryCoordinates(flags map[string]FlagValue) error {
	for _, name := range []string{"catalog", "repo", "command-id"} {
		if _, err := requireRemoteFlag(flags, name); err != nil {
			return err
		}
	}
	return nil
}
